package helper

import (
	"os"
	"strings"
)

// IsDebugging will return true if the process was launched from Delve or the
// gopls language server debugger.
//
// It does not detect situations where a debugger attached after process start.
func IsDevelopment() (bool, error) {
	executablePath, err := os.Executable()
	if err != nil {
		return false, err
	}
	if strings.Contains(strings.ToLower(executablePath), "tmp") {
		return true, nil
	}
	return false, nil
	// pid := os.Getppid()

	// // We loop in case there were intermediary processes like the gopls language server.
	// for pid != 0 {
	// 	switch p, err := ps.FindProcess(pid); {
	// 	case err != nil:
	// 		return false
	// 	case p.Executable() == "dlv":
	// 		return true
	// 	default:
	// 		pid = p.PPid()
	// 	}
	// }
	// return false
}
