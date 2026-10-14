package event

// WO-2026-10-14-013 Fase B: terminate harus membunuh process tree,
// idempoten, cepat, dan status akhir tetap terminate.

import (
	"os/exec"
	"syscall"
	"testing"
	"time"

	"job_item/support"
)

func waitForProcess(item *JobManagerEventItem, cmd *exec.Cmd, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		item.statusMu.Lock()
		started := item.started
		item.statusMu.Unlock()
		if started {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	item.statusMu.Lock()
	started := item.started
	item.statusMu.Unlock()
	return started
}

func TestWatchProcessCMD_TerminateKillsTreeAndKeepsStatus(t *testing.T) {
	st := setupEventTest(t)
	t.Setenv("JOB_ITEM_TERMINATE_GRACE_SECONDS", "1")

	item := &JobManagerEventItem{event: "test.term"}
	taskID := "task-term-tree"
	cmd := exec.Command("bash", "-c", "sleep 30; echo done")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 2 * time.Second

	done := make(chan struct{})
	go func() {
		item.WatchProcessCMD(cmd, taskID)
		close(done)
	}()
	if !waitForProcess(item, cmd, 5*time.Second) {
		t.Fatalf("process did not start")
	}
	time.Sleep(300 * time.Millisecond)

	// Simulasi callback _worker: catat intent terminate lalu tembak EventBus.
	item.setIntentStatus(GetStatus().STATUS_TERMINATE)
	start := time.Now()
	support.Helper.EventBus.GetBus().Publish(taskID + "_terminate")
	// Terminate ganda harus aman (idempoten).
	support.Helper.EventBus.GetBus().Publish(taskID + "_terminate")

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("WatchProcessCMD hung after terminate")
	}
	elapsed := time.Since(start)
	if elapsed > 5*time.Second {
		t.Fatalf("terminate too slow: %s", elapsed)
	}
	if got := item.lastStatus(); got != GetStatus().STATUS_TERMINATE {
		t.Fatalf("expected broker status terminate, got %q", got)
	}
	got, found, err := st.Get(taskID)
	if err != nil || !found {
		t.Fatalf("state not recorded: found=%v err=%v", found, err)
	}
	if got.Status != support.ProcessStatusTerminated {
		t.Fatalf("expected store TERMINATED, got %+v", got)
	}
	// Kontrak check_status Fase 1 tetap kompatibel: TERMINATED dipetakan ke FAILED (SPEC D-7).
	if status, _ := lookupJobStatus(ControlRequest{Action: ControlActionCheckStatus, RequestID: "req-1", JobID: taskID, Event: "test.term"}); status != support.ProcessStatusFailed {
		t.Fatalf("expected control response FAILED for TERMINATED, got %q", status)
	}
}
