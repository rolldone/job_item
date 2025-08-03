//go:build linux
// +build linux

package support

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
)

func ReplaceApp(executablePath string, app_new string) *exec.Cmd {
	cmd := exec.Command("sh", "-c", "rm "+executablePath+" || true && mv "+app_new+" "+executablePath+" && exit")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
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

// RunExecsProcess runs all exec commands defined in the configuration.
// It captures their output, retries on failure, and handles timeouts.
func (c *ConfigYamlSupport) RunExecsProcess() []*exec.Cmd {
	var cmd []*exec.Cmd
	retryCount := 5               // Number of retry attempts
	retryDelay := 2 * time.Second // Delay between retries

	for _, execConfig := range c.ConfigData.Execs {
		Helper.PrintGroupName(fmt.Sprintf("Running exec: %s, Key: %s, Cmd: %s\n", execConfig.Name, execConfig.Key, execConfig.Cmd))

		// Resolve working directory
		workingDir := execConfig.Working_dir
		if !filepath.IsAbs(workingDir) {
			workingDir = filepath.Join(filepath.Dir(c.Config_path), workingDir)
		}

		for attempt := 1; attempt <= retryCount; attempt++ {
			if runtime.GOOS == "windows" {
				cmd = append(cmd, exec.Command("cmd", "/C", execConfig.Cmd))
			} else {
				cmd = append(cmd, exec.Command("sh", "-c", execConfig.Cmd))
			}

			// Make the process independent by setting SysProcAttr
			cmd[len(cmd)-1].SysProcAttr = &syscall.SysProcAttr{
				Setpgid: true,
			}
			envInvolve := append(os.Environ(), "")
			cmd[len(cmd)-1].Env = envInvolve // Set the environment variables
			cmd[len(cmd)-1].Dir = workingDir // Set the working directory

			// Set environment variables
			for key, value := range execConfig.Env {
				cmd[len(cmd)-1].Env = append(cmd[len(cmd)-1].Env, fmt.Sprintf("%s=%s", key, value))
			}

			// Capture stdout and stderr
			stdout, err := cmd[len(cmd)-1].StdoutPipe()
			if err != nil {
				Helper.PrintErrName(fmt.Sprintf("Error creating stdout pipe for %s: %v\n", execConfig.Name, err))
				cmd = cmd[:len(cmd)-1] // Remove the last command if there's an error
				continue
			}
			stderr, err := cmd[len(cmd)-1].StderrPipe()
			if err != nil {
				Helper.PrintErrName(fmt.Sprintf("Error creating stderr pipe for %s: %v\n", execConfig.Name, err))
				cmd = cmd[:len(cmd)-1] // Remove the last command if there's an error
				continue
			}

			err = cmd[len(cmd)-1].Start()
			if err != nil {
				Helper.PrintErrName(fmt.Sprintf("Error starting command for %s (Attempt %d/%d): %v\n", execConfig.Name, attempt, retryCount, err))
				cmd = cmd[:len(cmd)-1] // Remove the last command if there's an error
				if attempt == retryCount {
					Helper.PrintErrName(fmt.Sprintf("Failed to execute command for %s after %d attempts\n", execConfig.Name, retryCount))
				}
				if attempt < retryCount {
					Helper.PrintGroupName(fmt.Sprintf("Retrying command for %s after %v...\n", execConfig.Name, retryDelay))
					time.Sleep(retryDelay) // Add delay before retrying
				}
				continue
			}

			// Wait for the command to finish with a timeout
			err = waitWithTimeout(cmd[len(cmd)-1], 2*time.Second)
			if err != nil {
				Helper.PrintErrName(fmt.Sprintf("Error waiting for command for %s (Attempt %d/%d): %v\n", execConfig.Name, attempt, retryCount, err))
				if attempt == retryCount {
					Helper.PrintErrName(fmt.Sprintf("Failed to execute command for %s after %d attempts\n", execConfig.Name, retryCount))
				}
				if attempt < retryCount {
					Helper.PrintGroupName(fmt.Sprintf("Retrying command for %s after %v...\n", execConfig.Name, retryDelay))
					time.Sleep(retryDelay) // Add delay before retrying
				}
				continue
			}

			go printOutputWithIdentity(stdout, execConfig.Name)
			go printOutputWithIdentity(stderr, execConfig.Name)
			break // Exit retry loop on success
		}
	}
	return cmd
}
