package support

// ProcessStateStore: persistensi state proses job per instance job_item (WO-2026-10-09-001, Task 2).
//
// Desain (mengacu spec.md FR-6):
//   - Satu SQLite per instance, satu tabel `process_state`, PK `job_id`.
//   - Driver CGO-free `modernc.org/sqlite` (build.sh memakai CGO_ENABLED=0). Pakai database/sql
//     langsung, TANPA GORM, agar job_item tetap ringan.
//   - WAL + busy_timeout untuk keamanan multi-connection dalam satu proses (single-writer).
//   - Lokasi default: `<dir config>/.job_item_data/state.db`. Karena ConfigYamlSupport melakukan
//     os.Chdir ke folder config, "dir config" == working directory proses. Prioritas override:
//     env JOB_ITEM_DATA_DIR > field YAML `data_dir` > argumen eksplisit pada Init.
//   - Folder dibuat dengan mode 0700.
//
// Waktu disimpan sebagai UTC RFC3339 (kolom TEXT) agar perbandingan leksikografis == kronologis,
// sehingga memudahkan kueri retensi pada Cleanup.

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // CGO-free SQLite driver
)

// Status state proses yang dipahami store. Nilai ini yang dibalas pada control topic (Task 4).
const (
	ProcessStatusRunning    = "RUNNING"
	ProcessStatusFinished   = "FINISHED"
	ProcessStatusFailed     = "FAILED"
	ProcessStatusTerminated = "TERMINATED"
)

// ProcessState merepresentasikan satu baris process_state.
type ProcessState struct {
	JobID      string  `json:"job_id"`
	Event      string  `json:"event"`
	Status     string  `json:"status"`
	StartedAt  string  `json:"started_at"`
	UpdatedAt  string  `json:"updated_at"`
	FinishedAt *string `json:"finished_at,omitempty"`
	ExitCode   int     `json:"exit_code"`
	LastLogAt  *string `json:"last_log_at,omitempty"`
	LastError  *string `json:"last_error,omitempty"`
}

// ProcessStateStore membungkus koneksi SQLite untuk satu instance job_item.
type ProcessStateStore struct {
	db     *sql.DB
	path   string
	closed bool
}

// ErrStoreClosed dikembalikan bila operasi dilakukan setelah Close.
var ErrStoreClosed = errors.New("process state store already closed")

// DefaultDataDirName adalah nama folder data default di dalam folder config.
const DefaultDataDirName = ".job_item_data"

// DefaultDBFileName adalah nama file SQLite default.
const DefaultDBFileName = "state.db"

// defaultBusyTimeoutMs menahan writer lain (dalam proses) agar menunggu lock, bukan gagal.
const defaultBusyTimeoutMs = 5000

var processStateStore *ProcessStateStore

// resolveDataDir menentukan folder data store dengan prioritas:
//  1. env JOB_ITEM_DATA_DIR (menang atas konfigurasi file)
//  2. config `data_dir` pada config.yaml (bila di-set)
//  3. default: <working dir>/.job_item_data
func resolveDataDir(dataDirOverride string) string {
	if env := os.Getenv("JOB_ITEM_DATA_DIR"); env != "" {
		return env
	}
	if Helper != nil && Helper.ConfigYaml != nil && Helper.ConfigYaml.ConfigData.Data_dir != "" {
		return Helper.ConfigYaml.ConfigData.Data_dir
	}
	if dataDirOverride != "" {
		return dataDirOverride
	}
	return DefaultDataDirName
}

// InitProcessStateStore membuka (dan membuat bila perlu) SQLite store di folder data.
// Prioritas folder: env JOB_ITEM_DATA_DIR > config `data_dir` > argumen dataDirOverride > default.
func InitProcessStateStore(dataDirOverride string) (*ProcessStateStore, error) {
	dataDir := resolveDataDir(dataDirOverride)
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data dir %s: %w", dataDir, err)
	}
	dbPath := filepath.Join(dataDir, DefaultDBFileName)

	// _pragma di DSN driver modernc untuk mengaktifkan WAL + busy_timeout sejak koneksi dibuat.
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(%d)&_pragma=synchronous(NORMAL)", dbPath, defaultBusyTimeoutMs)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", dbPath, err)
	}
	// Single-writer dalam proses: batasi 1 koneksi agar tidak ada lock contention antar goroutine.
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite %s: %w", dbPath, err)
	}

	st := &ProcessStateStore{db: db, path: dbPath}
	if err := st.migrate(); err != nil {
		db.Close()
		return nil, err
	}

	processStateStore = st
	return st, nil
}

// RegisterProcessStateStore memasang instance store global (dipakai Task 3/4 via ProcessState()).
func RegisterProcessStateStore(st *ProcessStateStore) {
	processStateStore = st
}

// ProcessStateStoreGlobal mengembalikan store aktif (nil bila belum di-Init).
func ProcessStateStoreGlobal() *ProcessStateStore {
	return processStateStore
}

func (s *ProcessStateStore) migrate() error {
	const q = `
CREATE TABLE IF NOT EXISTS process_state (
	job_id      TEXT PRIMARY KEY,
	event       TEXT NOT NULL,
	status      TEXT NOT NULL,
	started_at  TEXT NOT NULL,
	updated_at  TEXT NOT NULL,
	finished_at TEXT,
	exit_code   INTEGER DEFAULT -1,
	last_log_at TEXT,
	last_error  TEXT
);`
	if _, err := s.db.Exec(q); err != nil {
		return fmt.Errorf("migrate process_state: %w", err)
	}
	return nil
}

// Path mengembalikan lokasi file SQLite (untuk log/debug).
func (s *ProcessStateStore) Path() string { return s.path }

// stateTimeLayout adalah UTC fixed-width (9 digit fraksi, tidak di-trim) sehingga urutan
// leksikografis string == urutan kronologis. Dipakai untuk semua kolom waktu TEXT di store.
const stateTimeLayout = "2006-01-02T15:04:05.000000000Z07:00"

func nowUTC() string { return time.Now().UTC().Format(stateTimeLayout) }

// SetRunning menyimpan/men-reset record menjadi RUNNING (dipanggil saat proses dimulai).
// Mengembalikan konflik job_id yang sama menjadi fresh RUNNING: started/updated=now, final di-clear.
func (s *ProcessStateStore) SetRunning(jobID, event string) error {
	if s.closed {
		return ErrStoreClosed
	}
	if jobID == "" {
		return errors.New("SetRunning: empty job_id")
	}
	now := nowUTC()
	const q = `
INSERT INTO process_state (job_id, event, status, started_at, updated_at, finished_at, exit_code, last_log_at, last_error)
VALUES (?1, ?2, ?3, ?4, ?4, NULL, -1, ?4, NULL)
ON CONFLICT(job_id) DO UPDATE SET
	event       = excluded.event,
	status      = excluded.status,
	started_at  = excluded.started_at,
	updated_at  = excluded.updated_at,
	finished_at = NULL,
	exit_code   = -1,
	last_log_at = excluded.last_log_at,
	last_error  = NULL;`
	_, err := s.db.Exec(q, jobID, event, ProcessStatusRunning, now)
	if err != nil {
		return fmt.Errorf("SetRunning %s: %w", jobID, err)
	}
	return nil
}

// UpdateActivity memperbarui updated_at + last_log_at untuk record RUNNING.
// Tidak menyentuh status final (WHERE status = RUNNING) sesuai FR-7.
func (s *ProcessStateStore) UpdateActivity(jobID string) error {
	if s.closed {
		return ErrStoreClosed
	}
	now := nowUTC()
	const q = `UPDATE process_state SET updated_at = ?2, last_log_at = ?2 WHERE job_id = ?1 AND status = ?3;`
	_, err := s.db.Exec(q, jobID, now, ProcessStatusRunning)
	if err != nil {
		return fmt.Errorf("UpdateActivity %s: %w", jobID, err)
	}
	return nil
}

// SetFinal menyimpan status final (FINISHED/FAILED/TERMINATED) beserta exit_code dan last_error.
// Bila record belum ada (mis. store baru diaktifkan saat proses sudah berjalan), row dibuat.
// started_at diisi now bila insert baru; tidak menimpa nilai existing pada update.
func (s *ProcessStateStore) SetFinal(jobID, event, status string, exitCode int, lastError string) error {
	if s.closed {
		return ErrStoreClosed
	}
	if jobID == "" {
		return errors.New("SetFinal: empty job_id")
	}
	if status != ProcessStatusFinished && status != ProcessStatusFailed && status != ProcessStatusTerminated {
		return fmt.Errorf("SetFinal: status %q is not final", status)
	}
	now := nowUTC()
	var leSQL any
	if lastError != "" {
		leSQL = lastError
	}
	const q = `
INSERT INTO process_state (job_id, event, status, started_at, updated_at, finished_at, exit_code, last_log_at, last_error)
VALUES (?1, ?2, ?3, ?4, ?4, ?4, ?5, ?4, ?6)
ON CONFLICT(job_id) DO UPDATE SET
	status      = excluded.status,
	updated_at  = excluded.updated_at,
	finished_at = excluded.finished_at,
	exit_code   = excluded.exit_code,
	last_error  = COALESCE(excluded.last_error, process_state.last_error);`
	_, err := s.db.Exec(q, jobID, event, status, now, exitCode, leSQL)
	if err != nil {
		return fmt.Errorf("SetFinal %s: %w", jobID, err)
	}
	return nil
}

// Get mengambil state untuk jobID. found=false berarti tidak ada record (→ NOT_FOUND di Task 4).
func (s *ProcessStateStore) Get(jobID string) (st ProcessState, found bool, err error) {
	if s.closed {
		return st, false, ErrStoreClosed
	}
	const q = `SELECT job_id, event, status, started_at, updated_at, finished_at, exit_code, last_log_at, last_error FROM process_state WHERE job_id = ?1;`
	row := s.db.QueryRow(q, jobID)
	err = row.Scan(&st.JobID, &st.Event, &st.Status, &st.StartedAt, &st.UpdatedAt, &st.FinishedAt, &st.ExitCode, &st.LastLogAt, &st.LastError)
	if errors.Is(err, sql.ErrNoRows) {
		return st, false, nil
	}
	if err != nil {
		return st, false, fmt.Errorf("Get %s: %w", jobID, err)
	}
	return st, true, nil
}

// List mengembalikan semua state (untuk debug/cronjob lokal). Urut updated_at DESC.
func (s *ProcessStateStore) List() ([]ProcessState, error) {
	if s.closed {
		return nil, ErrStoreClosed
	}
	const q = `SELECT job_id, event, status, started_at, updated_at, finished_at, exit_code, last_log_at, last_error FROM process_state ORDER BY updated_at DESC;`
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, fmt.Errorf("List: %w", err)
	}
	defer rows.Close()
	out := []ProcessState{}
	for rows.Next() {
		var st ProcessState
		if err := rows.Scan(&st.JobID, &st.Event, &st.Status, &st.StartedAt, &st.UpdatedAt, &st.FinishedAt, &st.ExitCode, &st.LastLogAt, &st.LastError); err != nil {
			return nil, fmt.Errorf("List scan: %w", err)
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// Cleanup menghapus record final yang finished_at-nya lebih tua dari retention.
// Hanya menyentuh baris berstatus final + finished_at terisi (record RUNNING tidak pernah dihapus).
// Mengembalikan jumlah baris yang dihapus.
func (s *ProcessStateStore) Cleanup(retention time.Duration) (int64, error) {
	if s.closed {
		return 0, ErrStoreClosed
	}
	if retention < 0 {
		return 0, errors.New("Cleanup: negative retention")
	}
	cutoff := time.Now().UTC().Add(-retention).Format(stateTimeLayout)
	const q = `DELETE FROM process_state
WHERE status IN (?1, ?2, ?4)
  AND finished_at IS NOT NULL
  AND finished_at < ?3;`
	res, err := s.db.Exec(q, ProcessStatusFinished, ProcessStatusFailed, cutoff, ProcessStatusTerminated)
	if err != nil {
		return 0, fmt.Errorf("Cleanup: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// Close menutup koneksi DB. Setelah Close, operasi lain mengembalikan ErrStoreClosed.
func (s *ProcessStateStore) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
