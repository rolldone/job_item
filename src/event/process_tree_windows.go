//go:build windows

package event

import (
	"log"
	"os/exec"
	"strconv"
	"time"
)

func configureJobProcess(cmd *exec.Cmd) {}

func killProcessTree(cmd *exec.Cmd, taskID string) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	pid := cmd.Process.Pid
	err := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
	log.Printf("[TRACE-TERM] timestamp=%s stage=killProcessTree task_id=%q pid=%d method=taskkill err=%v", time.Now().UTC().Format(time.RFC3339Nano), taskID, pid, err)
	return err
}
