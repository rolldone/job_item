//go:build linux
// +build linux

package support

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"syscall"
)

func ReplaceApp(executablePath string, app_new string) *exec.Cmd {
	cmd := exec.Command("sh", "-c", "rm "+executablePath+" || true && mv "+app_new+" "+executablePath+" && exit")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}

func (c *ConfigYamlSupport) createForChildProcessCommand() (*exec.Cmd, error) {
	executablePath, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if c.child_process_app != nil {
		pwd, err := os.Getwd()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		executablePath = fmt.Sprint(pwd, "/", *c.child_process_app)
	}
	var cmd *exec.Cmd
	os_type := runtime.GOOS
	config_path := c.Config_path

	if config_path == "" {
		config_path = "config.yaml"
	}

	switch os_type {
	case "windows":
		cmd = exec.Command(executablePath, "child_process", "--config", config_path)
	case "darwin":
		cmd = exec.Command(executablePath, "child_process", "--config", config_path)
	case "linux":
		cmd = exec.Command(executablePath, "child_process", "--config", config_path)
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd, nil
}

func (c *ConfigYamlSupport) createForChildExecProcessCommand() (*exec.Cmd, error) {
	executablePath, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if c.child_process_app != nil {
		pwd, err := os.Getwd()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		executablePath = fmt.Sprint(pwd, "/", *c.child_process_app)
	}
	var cmd *exec.Cmd
	config_path := c.Config_path

	if config_path == "" {
		config_path = "config.yaml"
	}

	cmd = exec.Command(executablePath, "child_execs_process", "--config", config_path)

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	return cmd, nil
}

// createForExecCommand creates a command for execution with SysProcAttr.
func (c *ConfigYamlSupport) createForExecCommand(execConfig ExecConfig, workingDir string) *exec.Cmd {
	cmd := exec.Command("sh", "-c", execConfig.Cmd)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	cmd.Env = append(os.Environ(), c.GetEnvForExecProcess()...)
	cmd.Dir = workingDir

	// Set environment variables
	for key, value := range execConfig.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
	}

	return cmd
}

// CloseAllGroupProcesses kills each process in cmds.
// On Windows, it uses taskkill to ensure child processes are also killed.
// On Unix, it falls back to Process.Kill (or you can improve for group kill).
func (c *ConfigYamlSupport) CloseAllGroupProcesses(cmds []*exec.Cmd) {
	for _, cmdItem := range cmds {
		if cmdItem == nil || cmdItem.Process == nil {
			continue // Skip if cmdItem or its Process is nil
		}
		err := syscall.Kill(-cmdItem.Process.Pid, syscall.SIGTERM)
		if err != nil {
			Helper.PrintErrName("Error killing process: syscall.Kill() "+" - "+cmdItem.String()+" : "+err.Error(), "ERR-20230903201")
		}
	}
}
