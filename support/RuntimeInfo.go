package support

import (
	"os"
	"path/filepath"
	"runtime"
)

// RuntimeInfo describes where the worker process is running from. All paths are
// absolute and resolved through symlinks when possible. It is sent to the job
// manager so the admin UI can show the real location of the worker.
type RuntimeInfo struct {
	Working_dir     string `json:"working_dir"`
	Executable_path string `json:"executable_path"`
	Os              string `json:"os"`
	Arch            string `json:"arch"`
}

// resolveAbsolute returns an absolute, symlink-resolved path. When resolution is
// not possible the original value is returned so callers still get the best
// available information.
func resolveAbsolute(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	return abs
}

// GetRuntimePaths returns the absolute working directory and executable path of
// the current worker process.
func GetRuntimePaths() RuntimeInfo {
	exePath, _ := os.Executable()
	workingDir, _ := os.Getwd()
	return RuntimeInfo{
		Working_dir:     resolveAbsolute(workingDir),
		Executable_path: resolveAbsolute(exePath),
		Os:              runtime.GOOS,
		Arch:            runtime.GOARCH,
	}
}

// GetRuntimeInfo returns the runtime information of the current worker process.
func (c *HardwareInfoSupport) GetRuntimeInfo() RuntimeInfo {
	return GetRuntimePaths()
}

// ConfigJobSnapshot is the safe, redacted view of a configured job.
type ConfigJobSnapshot struct {
	Name     string `json:"name"`
	Event    string `json:"event"`
	Cmd      string `json:"cmd"`
	Pub_type string `json:"pub_type"`
}

// ConfigSnapshot returns a copy of the relevant configuration that is safe to
// send to the manager. It is built explicitly so the credential block
// (project_id / secret_key) can never leak.
func (c *ConfigYamlSupport) ConfigSnapshot() map[string]interface{} {
	if c == nil {
		return nil
	}
	jobs := make([]ConfigJobSnapshot, 0, len(c.ConfigData.Jobs))
	for _, job := range c.ConfigData.Jobs {
		jobs = append(jobs, ConfigJobSnapshot{
			Name:     job.Name,
			Event:    job.Event,
			Cmd:      job.Cmd,
			Pub_type: job.Pub_type,
		})
	}
	return map[string]interface{}{
		"end_point": c.ConfigData.End_point,
		"jobs":      jobs,
	}
}

// GetConfigSnapshot is a nil-safe helper for callers that may not have a
// constructed ConfigYamlSupport yet.
func GetConfigSnapshot(c *ConfigYamlSupport) map[string]interface{} {
	if c == nil {
		return nil
	}
	return c.ConfigSnapshot()
}
