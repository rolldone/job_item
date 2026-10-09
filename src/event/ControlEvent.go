package event

// ControlEvent: listener control topic untuk memverifikasi status proses job (WO-2026-10-09-001, Task 4).
//
// Kontrak (spec.md section 6):
//   - Subscribe `BasicSub` ke `<identity_id>.control` (broadcast per-worker; satu topik per worker
//     sehingga instance lain tidak ikut mengonsumsi — lihat hasil spike Task 1).
//   - Request : { action: "check_status", request_id, job_id, event }
//   - Response: dipublish ke `<request_id>.control_response` dengan payload section 6.2.
//
// Sumber status: `ProcessStateStore` (persisten, FR-6). Bila store tidak tersedia
// (mis. init gagal saat persistence dinonaktifkan), dipakai fallback registry memori
// per-instance; bila keduanya tidak punya record -> `NOT_FOUND`.

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"job_item/support"
)

// ControlActionCheckStatus adalah satu-satunya action yang ditangani listener.
const ControlActionCheckStatus = "check_status"

// ControlStatusNotFound menandakan job tidak ditemukan pada catatan worker ini.
// Catatan: ini BUKAN status final proses; jangan menandai job sebagai gagal (spec FR-4).
const ControlStatusNotFound = "NOT_FOUND"

// ControlRequest adalah payload request pada `<identity_id>.control`.
type ControlRequest struct {
	Action    string `json:"action"`
	RequestID string `json:"request_id"`
	JobID     string `json:"job_id"`
	Event     string `json:"event"`
}

// ControlResponse adalah payload response pada `<request_id>.control_response` (spec 6.2).
type ControlResponse struct {
	RequestID  string `json:"request_id"`
	JobID      string `json:"job_id"`
	IdentityID string `json:"identity_id"`
	Status     string `json:"status"`
	Event      string `json:"event"`
	CheckedAt  string `json:"checked_at"`
}

// ---------------------------------------------------------------------------
// Fallback registry memori per-instance (task_id -> status).
// Hanya dipakai bila store persisten tidak tersedia; store tetap sumber utama (FR-6).
// ---------------------------------------------------------------------------

var controlRegistry sync.Map

func registrySet(taskID, status string) {
	if taskID == "" {
		return
	}
	controlRegistry.Store(taskID, status)
}

func registryStatus(taskID string) (string, bool) {
	if taskID == "" {
		return "", false
	}
	if v, ok := controlRegistry.Load(taskID); ok {
		if s, ok := v.(string); ok {
			return s, true
		}
	}
	return "", false
}

// ---------------------------------------------------------------------------
// Helper persistensi terpadu: selalu mencatat ke registry memori, dan ke store bila tersedia.
// Dipakai dari siklus proses (JobManagerEvent.go) agar fallback tetap terisi.
// ---------------------------------------------------------------------------

// recordRunning menandai proses mulai: registry memori + store (bila aktif).
func recordRunning(taskID, eventName string) {
	registrySet(taskID, support.ProcessStatusRunning)
	if st := stateStoreInstance(); st != nil {
		if err := st.SetRunning(taskID, eventName); err != nil {
			log.Println("control :: SetRunning :: ", err)
		}
	}
}

// recordFinal menandai status final: registry memori + store (bila aktif).
func recordFinal(taskID, eventName, status string, exitCode int, lastError string) {
	registrySet(taskID, status)
	if st := stateStoreInstance(); st != nil {
		if err := st.SetFinal(taskID, eventName, status, exitCode, lastError); err != nil {
			log.Println("control :: SetFinal :: ", err)
		}
	}
}

// ---------------------------------------------------------------------------
// Listener
// ---------------------------------------------------------------------------

// listenControlTopic mendaftarkan BasicSub pada `<identity_id>.control`.
// Mengembalikan fungsi unsubscribe (no-op bila tidak bisa subscribe).
func (c *JobManagerEvent) listenControlTopic(conn support.BrokerConnectionInterface) func() {
	c.controlUnsubMu.Lock()
	defer c.controlUnsubMu.Unlock()
	if c.controlUnsub != nil {
		c.controlUnsub()
		c.controlUnsub = nil
	}
	if conn == nil {
		log.Println("control :: broker connection is nil, control listener disabled")
		return func() {}
	}
	identityID := support.Helper.ConfigYaml.ConfigData.Identity_id
	if identityID == "" {
		log.Println("control :: identity_id is empty, control listener disabled")
		return func() {}
	}

	topic := identityID + ".control"
	unsub, err := conn.BasicSub(topic, func(message string) {
		// Jangan biarkan payload rusak mematikan proses worker.
		defer func() {
			if r := recover(); r != nil {
				log.Println("control :: recovered from handler panic :: ", r)
			}
		}()
		handleControlMessage(conn, message)
	})
	if err != nil {
		log.Println("control :: error subscribing to", topic, "::", err)
		return func() {}
	}
	c.controlUnsub = unsub
	log.Println("control :: listening on", topic)
	return unsub
}

// handleControlMessage memproses satu pesan kontrol dan mempublikasikan response.
// Mengembalikan status yang dikirim (string kosong bila diabaikan) — memudahkan pengujian.
func handleControlMessage(conn support.BrokerConnectionInterface, message string) string {
	req := ControlRequest{}
	if err := json.Unmarshal([]byte(message), &req); err != nil {
		log.Println("control :: invalid payload, ignored ::", err)
		return ""
	}
	if req.Action != ControlActionCheckStatus {
		log.Println("control :: unknown action, ignored ::", req.Action)
		return ""
	}
	if req.RequestID == "" {
		log.Println("control :: missing request_id, ignored")
		return ""
	}
	if req.JobID == "" {
		log.Println("control :: missing job_id, ignored")
		return ""
	}

	status, eventName := lookupJobStatus(req)
	resp := ControlResponse{
		RequestID:  req.RequestID,
		JobID:      req.JobID,
		IdentityID: support.Helper.ConfigYaml.ConfigData.Identity_id,
		Status:     status,
		Event:      eventName,
		CheckedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	payload, err := json.Marshal(resp)
	if err != nil {
		log.Println("control :: failed to marshal response ::", err)
		return ""
	}
	conn.Pub(req.RequestID+".control_response", string(payload))
	return status
}

// lookupJobStatus menentukan status proses untuk sebuah request.
// Prioritas: store persisten (FR-6) -> fallback registry memori -> NOT_FOUND.
func lookupJobStatus(req ControlRequest) (status string, eventName string) {
	eventName = req.Event
	if st := support.ProcessStateStoreGlobal(); st != nil {
		if state, found, err := st.Get(req.JobID); err != nil {
			log.Println("control :: store lookup error ::", err)
			// Query failure: degrade to the in-memory registry if available.
		} else if found {
			if req.Event != "" && state.Event != "" && req.Event != state.Event {
				log.Printf("control :: event mismatch job=%s request=%s stored=%s", req.JobID, req.Event, state.Event)
			}
			return state.Status, state.Event
		} else {
			// Store healthy + row absent means NOT_FOUND (do not resurrect expired final state).
			return ControlStatusNotFound, eventName
		}
	}
	if s, ok := registryStatus(req.JobID); ok {
		return s, eventName
	}
	return ControlStatusNotFound, eventName
}
