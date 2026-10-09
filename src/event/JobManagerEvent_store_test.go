package event

// Integration test Task 3 (WO-2026-10-09-001): lifecycle store di WatchProcessCMD.
// Membuktikan: RUNNING dicatat, status final (FINISHED/FAILED) + exit_code + last_error
// tersimpan, dan proses yang gagal start tidak tertinggal berstatus RUNNING.

import (
	"os/exec"
	"strings"
	"sync"
	"testing"

	"job_item/support"
)

func setupEventTest(t *testing.T) *support.ProcessStateStore {
	t.Helper()
	// Minimal bootstrap: EventBus + ConfigYaml (End_point kosong -> tanpa publish broker).
	support.Helper = support.SupportConstruct("Test")
	support.Helper.EventBus = support.EventBusConstruct()
	support.Helper.ConfigYaml = &support.ConfigYamlSupport{}

	// Paksa store memakai folder temp dan reset Once agar stateStoreInstance menginisialisasi ulang.
	t.Setenv("JOB_ITEM_DATA_DIR", t.TempDir())
	stateStoreOnce = sync.Once{}
	stateStoreInst = nil

	st := stateStoreInstance()
	if st == nil {
		t.Fatalf("state store should be initialized")
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestWatchProcessCMD_RecordsFinalSuccess(t *testing.T) {
	st := setupEventTest(t)

	item := JobManagerEventItem{event: "test.success"}
	cmd := exec.Command("bash", "-c", "echo hello")
	item.WatchProcessCMD(cmd, "task-ok")

	got, found, err := st.Get("task-ok")
	if err != nil || !found {
		t.Fatalf("state not recorded: found=%v err=%v", found, err)
	}
	if got.Status != support.ProcessStatusFinished || got.ExitCode != 0 {
		t.Fatalf("expected FINISHED exit 0, got %+v", got)
	}
	if got.Event != "test.success" {
		t.Fatalf("event not propagated, got %q", got.Event)
	}
	if got.FinishedAt == nil || got.LastLogAt == nil {
		t.Fatalf("expected finished_at + last_log_at set: %+v", got)
	}
}

func TestWatchProcessCMD_RecordsFinalFailureWithError(t *testing.T) {
	st := setupEventTest(t)

	item := JobManagerEventItem{event: "test.fail"}
	cmd := exec.Command("bash", "-c", "echo BOOM >&2; exit 2")
	item.WatchProcessCMD(cmd, "task-fail")

	got, found, err := st.Get("task-fail")
	if err != nil || !found {
		t.Fatalf("state not recorded: found=%v err=%v", found, err)
	}
	if got.Status != support.ProcessStatusFailed || got.ExitCode != 2 {
		t.Fatalf("expected FAILED exit 2, got %+v", got)
	}
	if got.LastError == nil || !strings.Contains(*got.LastError, "BOOM") {
		t.Fatalf("expected last_error to contain BOOM, got %v", got.LastError)
	}
}

func TestWatchProcessCMD_StartFailureNotLeftRunning(t *testing.T) {
	st := setupEventTest(t)

	item := JobManagerEventItem{event: "test.startfail"}
	cmd := exec.Command("/nonexistent/binary-xyz")
	item.WatchProcessCMD(cmd, "task-startfail")

	got, found, err := st.Get("task-startfail")
	if err != nil || !found {
		t.Fatalf("state not recorded: found=%v err=%v", found, err)
	}
	if got.Status != support.ProcessStatusFailed || got.ExitCode != -1 {
		t.Fatalf("start failure must be FAILED exit -1, got %+v", got)
	}
}

// mockBroker mengimplementasikan support.BrokerConnectionInterface untuk mengamati urutan
// publish `_finish` relatif terhadap penulisan state final.
type mockBroker struct {
	pub func(topic, message string)
}

func (b *mockBroker) Pub(topic, message string) {
	if b.pub != nil {
		b.pub(topic, message)
	}
}
func (b *mockBroker) Sub(string, string, func(string)) (func(), error) { return func() {}, nil }
func (b *mockBroker) SubSync(string, string, func(string, error), support.SubSyncOpts) (bool, error) {
	return false, nil
}
func (b *mockBroker) GetBroker_P() any         { return b }
func (b *mockBroker) SetKey_P(string)          {}
func (b *mockBroker) GetKey_P() string         { return "mock" }
func (b *mockBroker) IsConnected() bool        { return true }
func (b *mockBroker) GetRefreshPubSub() string { return "refresh" }
func (b *mockBroker) BasicSub(string, func(string)) (func(), error) {
	return func() {}, nil
}
func (b *mockBroker) BasicSubSync(string, func(string, error), support.SubSyncOpts) (bool, error) {
	return false, nil
}

// TestRunGoroutine_FinalStateBeforeFinishPublish memverifikasi acceptance Task 3:
// saat `_finish` di-publish, store SUDAH berisi status final.
func TestRunGoroutine_FinalStateBeforeFinishPublish(t *testing.T) {
	st := setupEventTest(t)

	var (
		mu       sync.Mutex
		observed bool
		pubErr   error
	)
	broker := &mockBroker{}
	broker.pub = func(topic, _ string) {
		if !strings.HasSuffix(topic, "_finish") {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		jobID := strings.TrimSuffix(topic, "_finish")
		state, found, err := st.Get(jobID)
		if err != nil {
			pubErr = err
			return
		}
		observed = found && state.Status == support.ProcessStatusFinished && state.ExitCode == 0
	}

	item := JobManagerEventItem{conn: broker}
	item.RunGoroutine("echo hello", "task-order", "test.order")

	mu.Lock()
	defer mu.Unlock()
	if pubErr != nil {
		t.Fatalf("error reading state during _finish publish: %v", pubErr)
	}
	if !observed {
		t.Fatal("_finish published before final state persisted in store")
	}
}
