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
