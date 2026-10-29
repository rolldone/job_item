package support

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("JOB_ITEM_DATA_DIR", dir)

	src := &ConfigYamlSupport{ConfigData: ConfigData{
		Credential:        Credential{Project_id: "project-1", Secret_key: "secret"},
		Uuid:              "uuid-1",
		Broker_connection: map[string]interface{}{"type": "nats", "key": "broker-1"},
		Jobs:              []ConfigJob{{Name: "job", Event: "event", Cmd: "echo {{task_id}}"}},
	}}
	if err := src.saveConfigCache(); err != nil {
		t.Fatalf("saveConfigCache: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, ConfigCacheFileName))
	if err != nil {
		t.Fatalf("stat cache: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("cache mode = %o, want 600", info.Mode().Perm())
	}

	dst := &ConfigYamlSupport{ConfigData: ConfigData{
		Credential: Credential{Project_id: "project-local", Secret_key: "local-secret"},
	}}
	age, err := dst.applyConfigCache()
	if err != nil {
		t.Fatalf("applyConfigCache: %v", err)
	}
	if age < 0 {
		t.Fatalf("cache age = %s, want >= 0", age)
	}
	if got := dst.ConfigData.Broker_connection["type"]; got != "nats" {
		t.Fatalf("broker type = %v, want nats", got)
	}
	if dst.ConfigData.Uuid != "uuid-1" {
		t.Fatalf("uuid = %q, want uuid-1", dst.ConfigData.Uuid)
	}
	if len(dst.ConfigData.Jobs) != 1 {
		t.Fatalf("jobs len = %d, want 1", len(dst.ConfigData.Jobs))
	}
	// Local fields must be preserved.
	if dst.ConfigData.Credential.Project_id != "project-local" {
		t.Fatalf("local credential overwritten: %q", dst.ConfigData.Credential.Project_id)
	}
}

func TestLoadServerConfigRefusesChild(t *testing.T) {
	t.Setenv("JOB_ITEM_CONFIG_DATA", "")
	t.Setenv("JOB_ITEM_CONFIG_SOURCE", "child")

	c := &ConfigYamlSupport{ConfigData: ConfigData{End_point: "http://127.0.0.1:1"}}
	err := c.loadServerCOnfig()
	if !errors.Is(err, ErrConfigFetchRefused) {
		t.Fatalf("err = %v, want ErrConfigFetchRefused", err)
	}
}

func TestLoadServerConfigUsesEnvData(t *testing.T) {
	t.Setenv("JOB_ITEM_CONFIG_SOURCE", "")
	t.Setenv("JOB_ITEM_CONFIG_DATA", `{"Identity_id":"id-env","Uuid":"uuid-env"}`)

	c := &ConfigYamlSupport{}
	if err := c.loadServerCOnfig(); err != nil {
		t.Fatalf("loadServerCOnfig: %v", err)
	}
	if c.Config_source != "env" {
		t.Fatalf("Config_source = %q, want env", c.Config_source)
	}
	if c.ConfigData.Identity_id != "id-env" {
		t.Fatalf("identity = %q, want id-env", c.ConfigData.Identity_id)
	}
}
