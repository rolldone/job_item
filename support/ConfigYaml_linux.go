//go:build linux
// +build linux

package support

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"time"
)

func ReplaceApp(executablePath string, app_new string) *exec.Cmd {
	cmd := exec.Command("sh", "-c", "rm "+executablePath+" || true && mv "+app_new+" "+executablePath+" && exit")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}

// createIndependentCommand creates an independent command with SysProcAttr.
func createIndependentCommand(execConfig ExecConfig, workingDir string) *exec.Cmd {
	cmd := exec.Command("sh", "-c", execConfig.Cmd)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	cmd.Env = append(os.Environ(), "")
	cmd.Dir = workingDir

	// Set environment variables
	for key, value := range execConfig.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
	}

	return cmd
}

// RunChildProcess starts a child process with the current configuration.
// It returns the command object and any error encountered.
func (c *ConfigYamlSupport) RunChildProcess() (*exec.Cmd, error) {
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

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Println("Error creating stdout pipe:", err)
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		fmt.Println("Error creating stderr pipe:", err)
		return nil, err
	}

	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	go printOutput(stdout)
	go printOutput(stderr)

	// Wait for the command to finish with a timeout
	err = waitWithTimeout(cmd, 2*time.Second)
	if err != nil {
		Helper.PrintErrName(fmt.Sprintf("Error waiting for command: %v\n", err))
		return nil, err
	}

	return cmd, nil
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
			Helper.PrintErrName("Error killing process: syscall.Kill() " + " - " + cmdItem.String() + " : " + err.Error())
		}
	}
}

// RunChildExecsProcess starts a child process for executing commands defined in the configuration.
// It waits for the process to finish and returns any error encountered.
func (c *ConfigYamlSupport) RunChildExecsProcess() (*exec.Cmd, error) {
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
		cmd = exec.Command(executablePath, "child_execs_process", "--config", config_path)
	case "darwin":
		cmd = exec.Command(executablePath, "child_execs_process", "--config", config_path)
	case "linux":
		cmd = exec.Command(executablePath, "child_execs_process", "--config", config_path)
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Println("Error creating stdout pipe:", err)
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		fmt.Println("Error creating stderr pipe:", err)
		return nil, err
	}

	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	go printOutput(stdout)
	go printOutput(stderr)

	// Wait for the command to finish
	// err = cmd.Wait()
	// if err != nil {
	// 	fmt.Println("Error waiting for command:", err)
	// 	return nil, err
	// }

	return cmd, nil
}
