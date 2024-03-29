//go:build linux
// +build linux

package support

import (
	"os/exec"
	"syscall"
)

func ReplaceApp(executablePath string, app_new string) *exec.Cmd {
	cmd := exec.Command("sh", "-c", "rm "+executablePath+" || true && mv "+app_new+" "+executablePath+" && exit")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}
