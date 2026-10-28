package support

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetConfigString(t *testing.T) {
	tests := []struct {
		name    string
		config  map[string]interface{}
		key     string
		want    string
		wantErr bool
	}{
		{name: "valid", config: map[string]interface{}{"key": "broker-1"}, key: "key", want: "broker-1"},
		{name: "nil config", config: nil, key: "key", wantErr: true},
		{name: "missing key", config: map[string]interface{}{}, key: "key", wantErr: true},
		{name: "nil value", config: map[string]interface{}{"key": nil}, key: "key", wantErr: true},
		{name: "wrong type", config: map[string]interface{}{"key": 123}, key: "key", wantErr: true},
		{name: "empty string", config: map[string]interface{}{"key": "  "}, key: "key", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetConfigString(tt.config, tt.key)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (value=%q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetConfigInt(t *testing.T) {
	tests := []struct {
		name    string
		config  map[string]interface{}
		want    int
		wantErr bool
	}{
		{name: "int", config: map[string]interface{}{"port": 4222}, want: 4222},
		{name: "float64", config: map[string]interface{}{"port": float64(4222)}, want: 4222},
		{name: "int64", config: map[string]interface{}{"port": int64(4222)}, want: 4222},
		{name: "missing", config: map[string]interface{}{}, wantErr: true},
		{name: "wrong type", config: map[string]interface{}{"port": "4222"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetConfigInt(tt.config, "port")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (value=%d)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestGetBrokerConnectionKey_NoPanic(t *testing.T) {
	tests := []struct {
		name    string
		config  map[string]interface{}
		wantErr bool
		wantKey string
	}{
		{name: "valid", config: map[string]interface{}{"key": "broker-key"}, wantKey: "broker-key"},
		{name: "nil map", config: nil, wantErr: true},
		{name: "type nil", config: map[string]interface{}{"key": nil}, wantErr: true},
		{name: "key not string", config: map[string]interface{}{"key": 42}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			support := ConfigYamlSupport{}
			support.ConfigData.Broker_connection = tt.config

			got, err := support.GetBrokerConnectionKey()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (value=%q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantKey {
				t.Fatalf("got %q, want %q", got, tt.wantKey)
			}
		})
	}
}

func TestGetTypeBrokerCon_ReturnsErrorInsteadOfPanic(t *testing.T) {
	support := ConfigYamlSupport{}

	if _, err := support.GetTypeBrokerCon(nil); err == nil {
		t.Fatal("expected error for nil config, got nil")
	}
	if _, err := support.GetTypeBrokerCon(map[string]interface{}{"type": nil}); err == nil {
		t.Fatal("expected error for nil type, got nil")
	}
	if _, err := support.GetTypeBrokerCon(map[string]interface{}{"type": "nats", "key": "k"}); err == nil {
		t.Fatal("expected error for missing host/name/port, got nil")
	}
	if _, err := support.GetTypeBrokerCon(map[string]interface{}{"type": "unknown"}); err == nil {
		t.Fatal("expected error for unsupported broker type, got nil")
	}
}

func TestLoadConfigYaml_ReturnsErrorWhenMissing(t *testing.T) {
	support := ConfigYamlSupport{Config_path: filepath.Join(t.TempDir(), "does-not-exist.yaml")}

	if err := support.LoadConfigYaml(); err == nil {
		t.Fatal("expected error for missing config file, got nil")
	}
}

func TestLoadConfigYaml_ReturnsErrorWhenInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("broker_connection: [not-a-map"), 0o600); err != nil {
		t.Fatalf("failed to prepare config file: %v", err)
	}
	support := ConfigYamlSupport{Config_path: path}

	if err := support.LoadConfigYaml(); err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}
