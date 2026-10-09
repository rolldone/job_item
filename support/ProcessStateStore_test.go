package support

// Unit test ProcessStateStore (WO-2026-10-09-001, Task 2 acceptance).
// Mencakup: SetRunning -> UpdateActivity -> SetFinal -> Get -> List -> Cleanup,
// plus persistensi lintas restart (Close lalu Init ulang pada file yang sama).

import (
	"path/filepath"
	"testing"
	"time"
)

func newStore(t *testing.T) *ProcessStateStore {
	t.Helper()
	st, err := InitProcessStateStore(t.TempDir())
	if err != nil {
		t.Fatalf("InitProcessStateStore: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestProcessStateStore_DataDirPrecedence(t *testing.T) {
	originalHelper := Helper
	t.Cleanup(func() { Helper = originalHelper })
	t.Setenv("JOB_ITEM_DATA_DIR", "")
	Helper = SupportConstruct("Test")
	Helper.ConfigYaml = &ConfigYamlSupport{ConfigData: ConfigData{Data_dir: "yaml-data"}}

	if got := resolveDataDir("explicit-data"); got != "yaml-data" {
		t.Fatalf("config data_dir must override argument: got %q", got)
	}
	t.Setenv("JOB_ITEM_DATA_DIR", "env-data")
	if got := resolveDataDir("explicit-data"); got != "env-data" {
		t.Fatalf("env must override config and argument: got %q", got)
	}
	Helper.ConfigYaml.ConfigData.Data_dir = ""
	t.Setenv("JOB_ITEM_DATA_DIR", "")
	if got := resolveDataDir("explicit-data"); got != "explicit-data" {
		t.Fatalf("explicit argument must be fallback before default: got %q", got)
	}
}

func TestProcessStateStore_CRUDLifecycle(t *testing.T) {
	st := newStore(t)

	// 1. Get sebelum ada record -> found=false.
	if _, found, err := st.Get("job-1"); err != nil || found {
		t.Fatalf("expected not found, got found=%v err=%v", found, err)
	}

	// 2. SetRunning membuat record RUNNING.
	if err := st.SetRunning("job-1", "event.run"); err != nil {
		t.Fatalf("SetRunning: %v", err)
	}
	got, found, err := st.Get("job-1")
	if err != nil || !found {
		t.Fatalf("after SetRunning not found: found=%v err=%v", found, err)
	}
	if got.Status != ProcessStatusRunning || got.Event != "event.run" {
		t.Fatalf("unexpected state: %+v", got)
	}
	if got.ExitCode != -1 || got.FinishedAt != nil {
		t.Fatalf("running row should have exit_code=-1 and no finished_at: %+v", got)
	}
	firstUpdated := got.UpdatedAt

	// 3. UpdateActivity maju updated_at + last_log_at, status tetap RUNNING.
	time.Sleep(12 * time.Millisecond) // hindari identic timestamp RFC3339Nano pada resolusi sama
	if err := st.UpdateActivity("job-1"); err != nil {
		t.Fatalf("UpdateActivity: %v", err)
	}
	got, _, err = st.Get("job-1")
	if err != nil {
		t.Fatalf("Get after activity: %v", err)
	}
	if got.Status != ProcessStatusRunning {
		t.Fatalf("activity must not change status, got %q", got.Status)
	}
	if got.LastLogAt == nil || *got.LastLogAt == "" {
		t.Fatalf("activity must set last_log_at, got %+v", got.LastLogAt)
	}
	if got.UpdatedAt <= firstUpdated {
		t.Fatalf("updated_at should advance: before=%q after=%q", firstUpdated, got.UpdatedAt)
	}

	// 4. UpdateActivity pada record non-RUNNING tidak boleh mengubah apa pun (FR-7 guard).
	if err := st.SetFinal("job-1", "event.run", ProcessStatusFinished, 0, ""); err != nil {
		t.Fatalf("SetFinal: %v", err)
	}
	finalBefore, _, _ := st.Get("job-1")
	time.Sleep(2 * time.Millisecond)
	if err := st.UpdateActivity("job-1"); err != nil {
		t.Fatalf("UpdateActivity on final: %v", err)
	}
	finalAfter, _, _ := st.Get("job-1")
	if finalAfter.Status != ProcessStatusFinished || finalAfter.UpdatedAt != finalBefore.UpdatedAt {
		t.Fatalf("UpdateActivity leaked into final row: before=%+v after=%+v", finalBefore, finalAfter)
	}

	// 5. Get record final: exit_code + finished_at terisi.
	gotFinal, _, err := st.Get("job-1")
	if err != nil {
		t.Fatalf("Get final job-1: %v", err)
	}
	if gotFinal.Status != ProcessStatusFinished || gotFinal.ExitCode != 0 || gotFinal.FinishedAt == nil {
		t.Fatalf("final row wrong: %+v", gotFinal)
	}

	// 6. FAILED dengan last_error.
	if err := st.SetRunning("job-2", "event.fail"); err != nil {
		t.Fatalf("SetRunning job-2: %v", err)
	}
	if err := st.SetFinal("job-2", "event.fail", ProcessStatusFailed, 2, "boom"); err != nil {
		t.Fatalf("SetFinal job-2: %v", err)
	}
	g2, _, err := st.Get("job-2")
	if err != nil {
		t.Fatalf("Get job-2: %v", err)
	}
	if g2.Status != ProcessStatusFailed || g2.ExitCode != 2 || g2.LastError == nil || *g2.LastError != "boom" {
		t.Fatalf("job-2 not failed correctly: %+v", g2)
	}

	// 7. SetFinal menolak status non-final.
	if err := st.SetFinal("job-3", "e", ProcessStatusRunning, 0, ""); err == nil {
		t.Fatalf("expected error for non-final status, got nil")
	}

	// 8. List mengembalikan keduanya.
	all, err := st.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(all))
	}
}

func TestProcessStateStore_PersistAcrossRestart(t *testing.T) {
	dir := t.TempDir()

	st1, err := InitProcessStateStore(dir)
	if err != nil {
		t.Fatalf("Init st1: %v", err)
	}
	if err := st1.SetRunning("job-x", "event.x"); err != nil {
		t.Fatalf("SetRunning: %v", err)
	}
	if err := st1.SetFinal("job-x", "event.x", ProcessStatusFinished, 0, ""); err != nil {
		t.Fatalf("SetFinal: %v", err)
	}
	dbPath := st1.Path()
	if filepath.Base(dbPath) != DefaultDBFileName {
		t.Fatalf("unexpected db path %q", dbPath)
	}
	if err := st1.Close(); err != nil {
		t.Fatalf("Close st1: %v", err)
	}

	// "Restart": Init ulang pada folder yang sama, data harus masih ada.
	st2, err := InitProcessStateStore(dir)
	if err != nil {
		t.Fatalf("Init st2: %v", err)
	}
	defer st2.Close()
	got, found, err := st2.Get("job-x")
	if err != nil || !found {
		t.Fatalf("state lost across restart: found=%v err=%v", found, err)
	}
	if got.Status != ProcessStatusFinished {
		t.Fatalf("persisted status wrong: %+v", got)
	}
}

func TestProcessStateStore_Cleanup(t *testing.T) {
	st := newStore(t)

	// Record final "lama" (finished_at di masa lalu) dan satu RUNNING.
	if err := st.SetFinal("old-1", "e", ProcessStatusFinished, 0, ""); err != nil {
		t.Fatalf("SetFinal old-1: %v", err)
	}
	if err := st.SetFinal("old-2", "e", ProcessStatusFailed, 1, "x"); err != nil {
		t.Fatalf("SetFinal old-2: %v", err)
	}
	if err := st.SetRunning("live", "e"); err != nil {
		t.Fatalf("SetRunning live: %v", err)
	}

	// Geser finished_at kedua record lama agar jauh di masa lalu (retention 1 jam).
	oldTime := time.Now().UTC().Add(-10 * time.Hour).Format(stateTimeLayout)
	if _, err := st.db.Exec(`UPDATE process_state SET finished_at = ?1 WHERE job_id IN (?2, ?3)`, oldTime, "old-1", "old-2"); err != nil {
		t.Fatalf("force old finished_at: %v", err)
	}

	removed, err := st.Cleanup(time.Hour)
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if removed != 2 {
		t.Fatalf("expected 2 removed, got %d", removed)
	}
	// RUNNING tidak boleh terhapus.
	if _, found, _ := st.Get("live"); !found {
		t.Fatalf("running row must survive cleanup")
	}
	if _, found, _ := st.Get("old-1"); found {
		t.Fatalf("old finished row must be removed")
	}
}
