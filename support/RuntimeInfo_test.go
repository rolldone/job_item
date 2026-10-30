package support

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetRuntimePathsAbsolute(t *testing.T) {
	info := GetRuntimePaths()
	if info.Working_dir == "" {
		t.Fatal("working_dir is empty")
	}
	if info.Executable_path == "" {
		t.Fatal("executable_path is empty")
	}
	if !filepath.IsAbs(info.Working_dir) {
		t.Fatalf("working_dir %q is not absolute", info.Working_dir)
	}
	if !filepath.IsAbs(info.Executable_path) {
		t.Fatalf("executable_path %q is not absolute", info.Executable_path)
	}
	if info.Os == "" || info.Arch == "" {
		t.Fatalf("os/arch should be populated, got %q/%q", info.Os, info.Arch)
	}
}

func TestConfigSnapshotRedactsCredential(t *testing.T) {
	c := &ConfigYamlSupport{ConfigData: ConfigData{
		End_point:  "https://manager.example",
		Credential: Credential{Project_id: "project-secret-id", Secret_key: "super-secret-key"},
		Jobs: []ConfigJob{
			{Name: "job-a", Event: "event-a", Cmd: "echo hi", Pub_type: "one_to_one"},
		},
	}}

	snapshot := c.ConfigSnapshot()
	if snapshot == nil {
		t.Fatal("snapshot is nil")
	}
	if snapshot["end_point"] != "https://manager.example" {
		t.Fatalf("end_point = %v", snapshot["end_point"])
	}
	jobs, ok := snapshot["jobs"].([]ConfigJobSnapshot)
	if !ok || len(jobs) != 1 {
		t.Fatalf("jobs = %#v", snapshot["jobs"])
	}
	if jobs[0].Name != "job-a" || jobs[0].Cmd != "echo hi" {
		t.Fatalf("job snapshot mismatch: %#v", jobs[0])
	}

	payload, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	text := string(payload)
	for _, leaked := range []string{"credential", "project-secret-id", "super-secret-key", "secret_key", "project_id"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("snapshot leaks %q: %s", leaked, text)
		}
	}
}

func TestNilConfigSnapshot(t *testing.T) {
	if snapshot := GetConfigSnapshot(nil); snapshot != nil {
		t.Fatalf("expected nil snapshot, got %#v", snapshot)
	}
}
