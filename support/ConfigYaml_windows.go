//go:build windows
// +build windows

package support

import (
	"os/exec"
)

func ReplaceApp(executablePath string, app_new string) *exec.Cmd {
	cmd := exec.Command("cmd", "del "+executablePath+" && move "+app_new+" "+executablePath)
	cmd.CombinedOutput()
	return cmd
}
