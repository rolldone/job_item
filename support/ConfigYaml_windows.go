//go:build windows
// +build windows

package support

import (
	"os/exec"
)

func ReplaceApp(executablePath string, app_new string) *exec.Cmd {
	cmd := exec.Command("cmd", "ping google.co.id && del "+executablePath+" && move "+app_new+" "+executablePath)
	return cmd
}
