//go:build !windows

package event

import (
	"log"
	"os/exec"
	"syscall"
	"time"
)

func configureJobProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killProcessTree(cmd *exec.Cmd, taskID string) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	pgid := cmd.Process.Pid
	err := syscall.Kill(-pgid, syscall.SIGTERM)
	log.Printf("[TRACE-TERM] timestamp=%s stage=killProcessTree task_id=%q pgid=%d method=sigterm err=%v", time.Now().UTC().Format(time.RFC3339Nano), taskID, pgid, err)
	if err != nil {
		return nil // process group already exited; idempotent
	}
	grace := terminateGracePeriod()
	if grace > 0 {
		deadline := time.Now().Add(grace)
		for time.Now().Before(deadline) {
			if err := syscall.Kill(-pgid, 0); err != nil {
				return nil
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	err = syscall.Kill(-pgid, syscall.SIGKILL)
	log.Printf("[TRACE-TERM] timestamp=%s stage=killProcessTree task_id=%q pgid=%d method=sigkill err=%v", time.Now().UTC().Format(time.RFC3339Nano), taskID, pgid, err)
	return err
}
