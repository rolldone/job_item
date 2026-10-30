// Package support provides utilities for managing job execution, broker connections, and configurations.
//
// This package includes:
// - Definitions for broker connection types (NATS, AMQP, Redis).
// - Configuration structures for jobs and execution settings.
// - Functions to load and manage YAML configurations.
// - Methods to execute child processes with retries and timeouts.
// - Helper functions for handling process output and environment variables.

package support

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"job_item/src/helper"
	support_helper "job_item/support/helper"
	"job_item/support/model"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"dario.cat/mergo"
	"gopkg.in/yaml.v3"
)

type NatsBrokerConnection struct {
	Name      string `yaml:"name"`
	Key       string `yaml:"key"`
	Type      string `yaml:"type"`
	Host      string `yaml:"host"`
	Port      int    `yaml:"port"`
	Auth_type string `yaml:"auth_type"`
	User      string `yaml:"user"`
	Password  string `yaml:"password"`
	Token     string `yaml:"token"`
	Secure    bool   `yaml:"secure"`
	CAFile    string `yaml:"ca_file"`
	CertFile  string `yaml:"cert_file"`
	KeyFile   string `yaml:"key_file"`
}

func (c NatsBrokerConnection) GetConnection() any {
	return c
}

type AMQP_BrokerConnection struct {
	Name     string `yaml:"name"`
	Key      string `yaml:"key"`
	Type     string `yaml:"type"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Exchange string `yaml:"exchange"`
	Secure   bool   `yaml:"secure"`
	CAFile   string `yaml:"ca_file"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

func (c AMQP_BrokerConnection) GetConnection() any {
	return c
}

type RedisBrokerConnection struct {
	Name     string `yaml:"name"`
	Key      string `yaml:"key"`
	Type     string `yaml:"type"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Password string `yaml:"password"`
	Db       int    `yaml:"db"`
	Secure   bool   `yaml:"secure"`
	CAFile   string `yaml:"ca_file"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

func (c RedisBrokerConnection) GetConnection() any {
	return c
}

type BrokerConInterface interface {
	GetConnection() any
}

type ConfigJob struct {
	// Local
	Name  string `yaml:"name"`
	Event string `yaml:"event"`
	Cmd   string `yaml:"cmd"`
	// Import
	Pub_type string
}

type Credential struct {
	Project_id string `yaml:"project_id"`
	Secret_key string `yaml:"secret_key"`
}

type ExecConfig struct {
	Name         string            `yaml:"name"`
	Key          string            `yaml:"key"`
	Cmd          string            `yaml:"cmd"`
	Env          map[string]string `yaml:"env"`
	Working_dir  string            `yaml:"working_dir"`  // Add Working_dir field
	Cascade_exit bool              `yaml:"cascade_exit"` // Add Cascade_exit field
	Attempt      int               `yaml:"attempt"`      // Add Attempt field
}

type ConfigData struct {
	// Unique Identity ID for the job item
	Identity_id string `yaml:"identity_id"`
	// Directory for local process state SQLite store (optional; env JOB_ITEM_DATA_DIR takes precedence).
	Data_dir string `yaml:"data_dir"`
	// End point for the job item service
	End_point string `yaml:"end_point"`
	// Credential for the job item
	Credential Credential `yaml:"credential"`
	// Import
	Uuid                    string
	Broker_connection       map[string]interface{} `json:"broker_connection,omitempty"`
	Project                 model.ProjectDataView
	Jobs                    []ConfigJob
	Execs                   []ExecConfig `json:"execs,omitempty"` // Add execs property
	Job_item_version_number int          `json:"job_item_version_number,omitempty"`
	Job_item_version        string       `json:"job_item_version,omitempty"`
	Job_item_link           string       `json:"job_item_link,omitempty"`
}

type ConfigYamlSupportConstructPropsType struct {
	Config_path string
}

func ConfigYamlSupportContruct(props ConfigYamlSupportConstructPropsType) (*ConfigYamlSupport, error) {
	gg, err := newConfigYamlSupportLocal(props)
	if err != nil {
		return nil, err
	}

	if gg.ConfigData.End_point != "" {
		err = gg.loadServerCOnfig()
		if err != nil {
			return nil, err
		}
	} else {
		gg.printGroupName("WARNING: End point is not set, using local config only")
		// If not requesting from server, set Uuid to Project_id
		gg.ConfigData.Uuid = gg.ConfigData.Credential.Project_id
	}
	return gg, nil
}

// newConfigYamlSupportLocal loads config.yaml + env vars without contacting the server.
func newConfigYamlSupportLocal(props ConfigYamlSupportConstructPropsType) (*ConfigYamlSupport, error) {
	gg := &ConfigYamlSupport{
		Config_path: filepath.Base(props.Config_path),
	}

	stat, err := os.Stat(filepath.Dir(props.Config_path))
	if err != nil {
		return nil, fmt.Errorf("error getting config directory stat: %w", err)
	}
	if stat.IsDir() {
		gg.printGroupName("Set working dir :: " + filepath.Dir(props.Config_path))
	} else {
		gg.printGroupName("Set working dir :: .")
	}

	err = os.Chdir(filepath.Dir(props.Config_path))
	if err != nil {
		return nil, fmt.Errorf("error changing working dir to %q: %w", filepath.Dir(props.Config_path), err)
	}

	if err := gg.LoadConfigYaml(); err != nil {
		return nil, err
	}
	gg.useEnvToYamlValue()
	return gg, nil
}

// ConfigYamlSupportContructLocalOnly reloads config.yaml + env vars without ever calling
// the server. Server-derived fields from base (broker connection, project, jobs, uuid,
// version) are carried over so a restart keeps the previously fetched configuration, and
// the worker identity is preserved across restarts.
func ConfigYamlSupportContructLocalOnly(props ConfigYamlSupportConstructPropsType, base ConfigData) (*ConfigYamlSupport, error) {
	gg, err := newConfigYamlSupportLocal(props)
	if err != nil {
		return nil, err
	}

	if base.Identity_id != "" && os.Getenv("JOB_ITEM_IDENTITY_ID") == "" {
		gg.ConfigData.Identity_id = base.Identity_id
	}
	if base.Broker_connection != nil {
		gg.ConfigData.Broker_connection = base.Broker_connection
	}
	if base.Project.Uuid != "" || len(base.Project.Job_datas) > 0 {
		gg.ConfigData.Project = base.Project
	}
	if len(base.Jobs) > 0 {
		gg.ConfigData.Jobs = base.Jobs
	}
	if base.Uuid != "" {
		gg.ConfigData.Uuid = base.Uuid
	}
	if base.Job_item_version_number != 0 {
		gg.ConfigData.Job_item_version_number = base.Job_item_version_number
	}
	if base.Job_item_version != "" {
		gg.ConfigData.Job_item_version = base.Job_item_version
	}
	if base.Job_item_link != "" {
		gg.ConfigData.Job_item_link = base.Job_item_link
	}
	if len(gg.ConfigData.Execs) == 0 && len(base.Execs) > 0 {
		gg.ConfigData.Execs = base.Execs
	}
	gg.Config_source = "local"
	gg.Config_cache_age = 0
	return gg, nil
}

func (c *ConfigYamlSupport) printGroupName(printText string) {
	if Helper == nil {
		fmt.Println("Main >> ", printText)
		return
	}
	fmt.Println(Helper.Segment_app, " >> ", printText)
}

type ConfigYamlSupport struct {
	ConfigData        ConfigData
	child_process_app *string
	Config_path       string
	// Config_source records where the last successful configuration came from:
	// "server", "cache", "env", or empty for a local-only configuration.
	Config_source string
	// Config_cache_age is how old the on-disk cache was when Config_source == "cache".
	Config_cache_age time.Duration
}

const (
	// ConfigCacheFileName is the last-known-good configuration cache inside the data dir.
	ConfigCacheFileName = "config_cache.json"

	defaultConfigHTTPTimeoutSeconds = 10
	defaultConfigCacheTTLSeconds    = 86400
)

// ErrConfigFetchRefused is returned when a child process tries to fetch configuration.
// Child processes must receive their configuration through JOB_ITEM_CONFIG_DATA.
var ErrConfigFetchRefused = errors.New("config fetch refused: child process must use JOB_ITEM_CONFIG_DATA")

// configCachePayload is the on-disk representation of the last-known-good config.
type configCachePayload struct {
	SavedAt time.Time  `json:"saved_at"`
	Config  ConfigData `json:"config"`
}

// configDurationEnvSeconds reads an integer seconds env var and falls back to def on
// empty/invalid/non-positive values, logging a warning through the receiver.
func (c *ConfigYamlSupport) configDurationEnvSeconds(name string, def int) time.Duration {
	raw := os.Getenv(name)
	if raw == "" {
		return time.Duration(def) * time.Second
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		c.printGroupName(fmt.Sprintf("WARNING: invalid %s=%q, falling back to %d seconds", name, raw, def))
		return time.Duration(def) * time.Second
	}
	return time.Duration(value) * time.Second
}

func (c *ConfigYamlSupport) configHTTPTimeout() time.Duration {
	return c.configDurationEnvSeconds("JOB_ITEM_CONFIG_HTTP_TIMEOUT_SECONDS", defaultConfigHTTPTimeoutSeconds)
}

func (c *ConfigYamlSupport) configCacheTTL() time.Duration {
	return c.configDurationEnvSeconds("JOB_ITEM_CONFIG_CACHE_TTL_SECONDS", defaultConfigCacheTTLSeconds)
}

// configDataDir resolves the cache directory with the same precedence as the process
// state store: env JOB_ITEM_DATA_DIR > YAML data_dir > default .job_item_data.
func (c *ConfigYamlSupport) configDataDir() string {
	if env := os.Getenv("JOB_ITEM_DATA_DIR"); env != "" {
		return env
	}
	if c.ConfigData.Data_dir != "" {
		return c.ConfigData.Data_dir
	}
	return DefaultDataDirName
}

func (c *ConfigYamlSupport) configCachePath() string {
	return filepath.Join(c.configDataDir(), ConfigCacheFileName)
}

// saveConfigCache persists ConfigData atomically (temp file + rename) with mode 0600.
func (c *ConfigYamlSupport) saveConfigCache() error {
	dir := c.configDataDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config cache dir %q: %w", dir, err)
	}

	payload := configCachePayload{SavedAt: time.Now().UTC(), Config: c.ConfigData}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal config cache: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".config_cache-*.tmp")
	if err != nil {
		return fmt.Errorf("create config cache temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		tmp.Close()
		os.Remove(tmpPath)
	}

	if err := tmp.Chmod(0o600); err != nil {
		cleanup()
		return fmt.Errorf("chmod config cache temp file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write config cache temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync config cache temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close config cache temp file: %w", err)
	}

	if err := os.Rename(tmpPath, c.configCachePath()); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("replace config cache: %w", err)
	}
	return nil
}

// applyConfigCache loads the last-known-good server fields from disk and merges them
// into the local configuration. It returns the cache age so callers can warn when the
// cache is older than TTL. Local fields (credential, end_point, data_dir) are preserved.
func (c *ConfigYamlSupport) applyConfigCache() (time.Duration, error) {
	path := c.configCachePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read config cache %q: %w", path, err)
	}

	var payload configCachePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return 0, fmt.Errorf("parse config cache %q: %w", path, err)
	}

	age := time.Since(payload.SavedAt)
	if payload.SavedAt.IsZero() {
		if info, statErr := os.Stat(path); statErr == nil {
			age = time.Since(info.ModTime())
		}
	}

	cached := payload.Config
	c.ConfigData.Broker_connection = cached.Broker_connection
	c.ConfigData.Project = cached.Project
	c.ConfigData.Jobs = cached.Jobs
	c.ConfigData.Uuid = cached.Uuid
	c.ConfigData.Job_item_version_number = cached.Job_item_version_number
	c.ConfigData.Job_item_version = cached.Job_item_version
	c.ConfigData.Job_item_link = cached.Job_item_link
	if len(cached.Execs) > 0 {
		c.ConfigData.Execs = cached.Execs
	}
	return age, nil
}

// RefreshCacheFromServer performs a single server fetch using a deep copy of the
// current configuration and only refreshes the on-disk cache. It never mutates the
// live configuration, so it is safe to call from a background goroutine.
func (c *ConfigYamlSupport) RefreshCacheFromServer() error {
	data, err := json.Marshal(c.ConfigData)
	if err != nil {
		return fmt.Errorf("clone config for refresh: %w", err)
	}
	var clone ConfigData
	if err := json.Unmarshal(data, &clone); err != nil {
		return fmt.Errorf("clone config for refresh: %w", err)
	}

	probe := &ConfigYamlSupport{ConfigData: clone, Config_path: c.Config_path}
	if err := probe.fetchAndMergeServerConfig(); err != nil {
		return err
	}
	return probe.saveConfigCache()
}

// LoadConfigYaml loads the configuration from a YAML file.
// It returns an error when the file cannot be read or parsed.
func (c *ConfigYamlSupport) LoadConfigYaml() error {
	yamlFile, err := os.ReadFile(c.Config_path)
	if err != nil {
		return fmt.Errorf("problem opening config file %q: %w", c.Config_path, err)
	}
	c.ConfigData = ConfigData{}

	// Generate a new UUID for the Identity_id
	identityId := os.Getenv("JOB_ITEM_IDENTITY_ID")
	if identityId == "" {
		idenityIdNew, err := support_helper.GenerateUUIDv7()
		if err != nil {
			return fmt.Errorf("failed to generate identity UUID: %w", err)
		}
		identityId = idenityIdNew
	}
	c.ConfigData.Identity_id = identityId

	err = yaml.Unmarshal(yamlFile, &c.ConfigData)
	if err != nil {
		return fmt.Errorf("failed to parse config file %q: %w", c.Config_path, err)
	}
	return nil
}

// useEnvToYamlValue updates the configuration values using environment variables.
func (c *ConfigYamlSupport) useEnvToYamlValue() {
	helper.Fromenv(&c.ConfigData)
}

func (c *ConfigYamlSupport) GetEnv() []string {
	baseURL := "http://localhost:"
	baseURL += strconv.Itoa(Helper.Gin.Port)

	configData, err := json.Marshal(c.ConfigData)
	if err != nil {
		c.printGroupName("Error marshalling ConfigData to JSON: " + err.Error())
		panic(1)
	}

	// Prepare share-data env values for child process
	shareBase := os.Getenv("JOB_ITEM_SHARE_DATA_BASE")
	if shareBase == "" {
		shareBase = "/msg/share/data"
	}
	// normalize baseURL
	if baseURL != "" {
		baseURL = strings.TrimRight(baseURL, "/")
	}
	shareHost := baseURL + shareBase

	jobItemShareDataAdd := shareHost + "/:task_id"
	jobItemShareDataGet := shareHost + "/:task_id/:key"

	return []string{
		// For child and child exec processes
		"JOB_ITEM_IDENTITY_ID=" + c.ConfigData.Identity_id,
		"JOB_ITEM_BASE_URL=" + baseURL,
		"JOB_ITEM_CONFIG_DATA=" + string(configData),
		// Explicit marker so a child process never tries to fetch config from the server.
		"JOB_ITEM_CONFIG_SOURCE=child",
		// Specific endpoints for child process to add/get share data
		"JOB_ITEM_SHARE_DATA_HOST=" + shareHost,
		"JOB_ITEM_SHARE_DATA_ADD=" + jobItemShareDataAdd,
		"JOB_ITEM_SHARE_DATA_GET=" + jobItemShareDataGet,
	}
}

// mergeEnv appends overrides to base, dropping any base entry that shares a key with an
// override so the override always wins inside the child process (duplicate keys are
// otherwise read first-wins by os.Getenv on Unix).
func mergeEnv(base []string, overrides []string) []string {
	keys := make(map[string]struct{}, len(overrides))
	for _, kv := range overrides {
		if idx := strings.IndexByte(kv, '='); idx >= 0 {
			keys[kv[:idx]] = struct{}{}
		}
	}

	merged := make([]string, 0, len(base)+len(overrides))
	for _, kv := range base {
		if idx := strings.IndexByte(kv, '='); idx >= 0 {
			if _, ok := keys[kv[:idx]]; ok {
				continue
			}
		}
		merged = append(merged, kv)
	}
	return append(merged, overrides...)
}

func (c *ConfigYamlSupport) GetEnvForExecProcess() []string {
	baseURL := os.Getenv("JOB_ITEM_BASE_URL")

	return []string{
		// For child exec processes, set the JOB_ITEM_XXX
		"JOB_ITEM_APP_ID=" + c.ConfigData.Uuid,
		"JOB_ITEM_BASE_URL=" + baseURL,
		"JOB_ITEM_CREATE_URL=" + baseURL + "/job/create",
		"JOB_ITEM_LISTEN_LOG=" + baseURL + "/job/listen/job_record/:job_record_uuid/log",
		"JOB_ITEM_LISTEN_NOTIF=" + baseURL + "/job/listen/job_record/:job_record_uuid/notif",
		"JOB_ITEM_LISTEN_ACTION=" + baseURL + "/job/job_record/:job_record_uuid/action/:action",
	}
}

// loadServerCOnfig requests the configuration from the server and updates the ConfigData.
// If the request fails and a cache exists, the cached server fields are applied and nil is
// returned so the worker can still start. A child process never performs a fetch.
func (c *ConfigYamlSupport) loadServerCOnfig() error {
	if os.Getenv("JOB_ITEM_CONFIG_DATA") != "" {
		// If JOB_ITEM_CONFIG_DATA is set, use it as the config data
		configDataString := os.Getenv("JOB_ITEM_CONFIG_DATA")
		var configData ConfigData
		err := json.Unmarshal([]byte(configDataString), &configData)
		if err != nil {
			return fmt.Errorf("error unmarshalling JOB_ITEM_CONFIG_DATA: %w", err)
		}
		c.ConfigData = configData
		c.Config_source = "env"
		c.printGroupName("Using JOB_ITEM_CONFIG_DATA from environment variable")
		return nil
	}

	// Explicit guard: a child process must never contact the config endpoint.
	if strings.EqualFold(os.Getenv("JOB_ITEM_CONFIG_SOURCE"), "child") {
		return ErrConfigFetchRefused
	}

	if err := c.fetchAndMergeServerConfig(); err != nil {
		age, cacheErr := c.applyConfigCache()
		if cacheErr != nil {
			return err
		}
		c.Config_source = "cache"
		c.Config_cache_age = age
		if ttl := c.configCacheTTL(); age > ttl {
			c.printGroupName(fmt.Sprintf(
				"WARNING: config cache is older than TTL (age=%s ttl=%s); using stale cache because the server is unreachable",
				age.Round(time.Second), ttl))
		}
		c.printGroupName(fmt.Sprintf(
			"config source=cache age=%s (server fetch failed: %v)", age.Round(time.Second), err))
		return nil
	}

	c.Config_source = "server"
	if err := c.saveConfigCache(); err != nil {
		c.printGroupName("WARNING: failed to persist config cache: " + err.Error())
	} else {
		c.printGroupName("config source=server (cache updated)")
	}
	return nil
}

// fetchAndMergeServerConfig performs POST {end_point}/api/worker/config with a bounded
// HTTP client and context, then merges the response into ConfigData.
func (c *ConfigYamlSupport) fetchAndMergeServerConfig() error {
	param := map[string]interface{}{
		"project_id": c.ConfigData.Credential.Project_id,
		"secret_key": c.ConfigData.Credential.Secret_key,
		"os":         runtime.GOOS,   // windows | darwin | linux
		"arch":       runtime.GOARCH, //  386, amd64, arm, s390x
	}

	hostInfo, err := Helper.HardwareInfo.GetInfoHardware()
	if err != nil {
		return fmt.Errorf("error getting hardware info: %w", err)
	}
	param["host"] = hostInfo
	// Absolute paths of the running worker + redacted config snapshot. Never
	// include the credential block here (see ConfigSnapshot).
	param["runtime"] = GetRuntimePaths()
	param["config_snapshot"] = c.ConfigSnapshot()

	jsonDataParam, err := json.Marshal(param)
	if err != nil {
		return fmt.Errorf("error marshalling config request: %w", err)
	}

	timeout := c.configHTTPTimeout()
	client := &http.Client{Timeout: timeout}
	endpoint := fmt.Sprint(c.ConfigData.End_point, "/api/worker/config")
	c.printGroupName("Attempting to connect to server: " + endpoint)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(jsonDataParam))
	if err != nil {
		c.printGroupName("Failed to create HTTP request to " + endpoint + " :: " + err.Error())
		return err
	}
	request.Header.Set("Content-Type", "application/json")

	c.printGroupName("Sending configuration request to server...")
	started := time.Now()
	response, err := client.Do(request)
	elapsed := time.Since(started)
	if err != nil {
		c.printGroupName("Connection failed to server " + endpoint + " after " + elapsed.Round(time.Millisecond).String())
		c.printGroupName("This could mean:")
		c.printGroupName("  - Server is down or unreachable")
		c.printGroupName("  - Network connectivity issues")
		c.printGroupName("  - Invalid endpoint URL")
		c.printGroupName("  - Firewall blocking the connection")
		c.printGroupName("Original error: " + err.Error())
		return err
	}
	defer response.Body.Close()

	// Check HTTP status code
	if response.StatusCode != http.StatusOK {
		c.printGroupName("ERROR: Server returned HTTP " + fmt.Sprint(response.StatusCode) + " status code after " + elapsed.Round(time.Millisecond).String())
		c.printGroupName("Expected 200 OK but got " + response.Status)
		return fmt.Errorf("server returned non-200 status: %d %s", response.StatusCode, response.Status)
	}

	c.printGroupName("Successfully connected to server (HTTP " + fmt.Sprint(response.StatusCode) + ", " + elapsed.Round(time.Millisecond).String() + ")")
	bodyData := struct {
		Return ConfigData
	}{}
	err = json.NewDecoder(response.Body).Decode(&bodyData)
	if err != nil {
		c.printGroupName("ERROR: Failed to parse server response JSON :: " + err.Error())
		c.printGroupName("This could mean the server returned invalid JSON format")
		return fmt.Errorf("failed to parse server response JSON: %w", err)
	}

	c.printGroupName("Configuration successfully received from server")
	// configData := bodyData["return"].(ConfigData)
	c.ConfigData.Broker_connection = bodyData.Return.Broker_connection
	c.ConfigData.Job_item_version_number = bodyData.Return.Job_item_version_number
	c.ConfigData.Job_item_link = bodyData.Return.Job_item_link
	mergo.Merge(&c.ConfigData, bodyData.Return)
	return nil
}

func (c *ConfigYamlSupport) DownloadNewApp(versionNumber int) error {

	url := c.ConfigData.Job_item_link
	outputPath := ""
	os_type := runtime.GOOS
	switch os_type {
	case "windows":
		outputPath = fmt.Sprint("job_item_", versionNumber, ".exe")
	case "darwin":
		outputPath = fmt.Sprint("job_item_", versionNumber)
	case "linux":
		outputPath = fmt.Sprint("job_item_", versionNumber)
	}
	c.child_process_app = &outputPath

	c.replaceApp(outputPath)

	// Check the file first is exist or not
	_, err := os.Stat(outputPath)
	if err == nil {
		return nil
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Send HTTP GET request
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check response status code
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned non-200 status code: %d", resp.StatusCode)
	}

	// Copy the response body to the output file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	// Make the downloaded file executable
	if os_type != "windows" {
		err = makeExecutable(outputPath)
		if err != nil {
			return err
		}
	}

	return nil
}

// replaceApp replaces the current application with a new version.
// It handles signals to ensure a graceful replacement process.
func (c *ConfigYamlSupport) replaceApp(app_new string) {
	go func() {
		defer func() {
			var cmd *exec.Cmd
			// os_type := runtime.GOOS
			fmt.Println("Replace the current app to be new version")
			executablePath, _ := os.Executable()
			cmd = ReplaceApp(executablePath, app_new)
			err := cmd.Run()
			if err != nil {
				fmt.Println(err)
			}
		}()
		// Create a channel to receive signals
		sigCh := make(chan os.Signal, 1)

		// Notify the sigCh channel for SIGINT and SIGTERM signals
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

		// Block until a signal is received
		sig := <-sigCh
		Helper.PrintGroupName(fmt.Sprintf("Received signal: %v\n", sig))
	}()
}

func makeExecutable(path string) error {
	cmd := exec.Command("chmod", "+x", path)
	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

// printOutput reads and prints the output from the provided reader.
func printOutput(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		fmt.Println(strings.TrimSpace(scanner.Text()))
	}
}

// printOutputWithIdentity reads and prints the output from the provided reader with an identity prefix.
func printOutputWithIdentity(reader io.Reader, identity string) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		Helper.PrintGroupName(fmt.Sprintf("[%s] >> %s", identity, strings.TrimSpace(scanner.Text())))
	}
}

// DownloadAndExtractCerts downloads a zip file from the backend and extracts it to the given directory.
func (c *ConfigYamlSupport) DownloadAndExtractCerts(endpoint, destDir, appID, appSecret string) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	// Prepare JSON body
	body := map[string]string{
		"app_id":     appID,
		"app_secret": appSecret,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return err
	}

	resp, err := http.Post(endpoint, "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download certs zip: status %d", resp.StatusCode)
	}

	// Read zip into memory
	zipBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	zipReader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return err
	}

	// Extract files from zip
	for _, f := range zipReader.File {
		outPath := filepath.Join(destDir, f.Name)
		if f.FileInfo().IsDir() {
			os.MkdirAll(outPath, 0755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return err
		}
		outFile, err := os.Create(outPath)
		if err != nil {
			return err
		}
		inFile, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}
		_, err = io.Copy(outFile, inFile)
		inFile.Close()
		outFile.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// GetConfigInt reads an integer value from a config map without panicking.
// int, int64 and float64 values are accepted.
func GetConfigInt(config map[string]interface{}, key string) (int, error) {
	if config == nil {
		return 0, fmt.Errorf("config is empty, cannot read %q", key)
	}
	raw, ok := config[key]
	if !ok || raw == nil {
		return 0, fmt.Errorf("config %q is not specified", key)
	}
	switch value := raw.(type) {
	case int:
		return value, nil
	case int64:
		return int(value), nil
	case float64:
		return int(value), nil
	default:
		return 0, fmt.Errorf("config %q has unexpected type %T (expected int)", key, raw)
	}
}

// optionalConfigString reads an optional string value from a config map.
// A missing or nil value returns an empty string without error.
func optionalConfigString(config map[string]interface{}, key string) (string, error) {
	if config == nil {
		return "", nil
	}
	raw, ok := config[key]
	if !ok || raw == nil {
		return "", nil
	}
	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("config %q has unexpected type %T (expected string)", key, raw)
	}
	return value, nil
}

// GetTypeBrokerCon returns the appropriate broker connection type based on the provided configuration.
// It returns an error instead of panicking when required values are missing or have the wrong type.
func (c *ConfigYamlSupport) GetTypeBrokerCon(v map[string]interface{}) (BrokerConInterface, error) {
	brokerType, err := GetConfigString(v, "type")
	if err != nil {
		return nil, err
	}
	host, err := GetConfigString(v, "host")
	if err != nil {
		return nil, err
	}
	key, err := GetConfigString(v, "key")
	if err != nil {
		return nil, err
	}
	name, err := GetConfigString(v, "name")
	if err != nil {
		return nil, err
	}
	port, err := GetConfigInt(v, "port")
	if err != nil {
		return nil, err
	}

	switch brokerType {
	case "nats":
		natsConf := NatsBrokerConnection{}
		natsConf.Host = host
		natsConf.Key = key
		natsConf.Name = name
		natsConf.Port = port
		natsConf.Type = brokerType
		if natsConf.Auth_type, err = optionalConfigString(v, "auth_type"); err != nil {
			return nil, err
		}
		if natsConf.Auth_type == "" {
			return nil, fmt.Errorf("NATS configuration error: auth_type must be defined [none | user_password | token] in the Job Manager")
		}
		if natsConf.User, err = optionalConfigString(v, "user"); err != nil {
			return nil, err
		}
		if natsConf.Password, err = optionalConfigString(v, "password"); err != nil {
			return nil, err
		}
		if natsConf.Token, err = optionalConfigString(v, "token"); err != nil {
			return nil, err
		}
		// Parse TLS fields
		if v["secure"] != nil {
			switch val := v["secure"].(type) {
			case bool:
				natsConf.Secure = val
			case float64:
				natsConf.Secure = val != 0
			case string:
				natsConf.Secure = val == "true" || val == "1"
			}
		}
		if natsConf.CAFile, err = optionalConfigString(v, "ca_file"); err != nil {
			return nil, err
		}
		if natsConf.CertFile, err = optionalConfigString(v, "cert_file"); err != nil {
			return nil, err
		}
		if natsConf.KeyFile, err = optionalConfigString(v, "key_file"); err != nil {
			return nil, err
		}
		if Helper.ConfigYaml.ConfigData.End_point != "" && natsConf.Secure && natsConf.CAFile != "" {
			certDir := filepath.Dir(natsConf.CAFile)
			endpoint := fmt.Sprintf("%s/api/worker/config/tls/download", c.ConfigData.End_point)
			if err := c.DownloadAndExtractCerts(endpoint, certDir, c.ConfigData.Credential.Project_id, c.ConfigData.Credential.Secret_key); err != nil {
				return nil, fmt.Errorf("NATS configuration error: failed to download TLS certificates: %w", err)
			}
		}
		return natsConf, nil
	case "rabbitmq":
		rabbitmqConf := AMQP_BrokerConnection{}
		rabbitmqConf.Host = host
		rabbitmqConf.Key = key
		rabbitmqConf.Name = name
		rabbitmqConf.Port = port
		rabbitmqConf.Type = brokerType
		if rabbitmqConf.User, err = optionalConfigString(v, "user"); err != nil {
			return nil, err
		}
		if rabbitmqConf.Password, err = optionalConfigString(v, "password"); err != nil {
			return nil, err
		}
		if rabbitmqConf.Exchange, err = optionalConfigString(v, "exchange"); err != nil {
			return nil, err
		}
		// Parse TLS/mTLS fields
		if v["secure"] != nil {
			switch val := v["secure"].(type) {
			case bool:
				rabbitmqConf.Secure = val
			case float64:
				rabbitmqConf.Secure = val != 0
			case string:
				rabbitmqConf.Secure = val == "true" || val == "1"
			}
		}
		if rabbitmqConf.CAFile, err = optionalConfigString(v, "ca_file"); err != nil {
			return nil, err
		}
		if rabbitmqConf.CertFile, err = optionalConfigString(v, "cert_file"); err != nil {
			return nil, err
		}
		if rabbitmqConf.KeyFile, err = optionalConfigString(v, "key_file"); err != nil {
			return nil, err
		}
		if Helper.ConfigYaml.ConfigData.End_point != "" && rabbitmqConf.Secure && rabbitmqConf.CAFile != "" {
			certDir := filepath.Dir(rabbitmqConf.CAFile)
			endpoint := fmt.Sprintf("%s/api/worker/config/tls/download", c.ConfigData.End_point)
			if err := c.DownloadAndExtractCerts(endpoint, certDir, c.ConfigData.Credential.Project_id, c.ConfigData.Credential.Secret_key); err != nil {
				return nil, fmt.Errorf("RabbitMQ configuration error: failed to download TLS certificates: %w", err)
			}
		}
		return rabbitmqConf, nil
	case "redis":
		redisConf := RedisBrokerConnection{}
		redisConf.Host = host
		redisConf.Key = key
		redisConf.Name = name
		redisConf.Port = port
		redisConf.Type = brokerType
		if redisConf.Password, err = optionalConfigString(v, "password"); err != nil {
			return nil, err
		}
		if redisConf.Password == "" {
			return nil, fmt.Errorf("REDIS configuration error: password must be defined in the Job Manager")
		}
		if redisConf.Db, err = GetConfigInt(v, "db"); err != nil {
			return nil, fmt.Errorf("REDIS configuration error: %w", err)
		}
		// Parse TLS/mTLS fields
		if v["secure"] != nil {
			switch val := v["secure"].(type) {
			case bool:
				redisConf.Secure = val
			case float64:
				redisConf.Secure = val != 0
			case string:
				redisConf.Secure = val == "true" || val == "1"
			}
		}
		if redisConf.CAFile, err = optionalConfigString(v, "ca_file"); err != nil {
			return nil, err
		}
		if redisConf.CertFile, err = optionalConfigString(v, "cert_file"); err != nil {
			return nil, err
		}
		if redisConf.KeyFile, err = optionalConfigString(v, "key_file"); err != nil {
			return nil, err
		}
		if Helper.ConfigYaml.ConfigData.End_point != "" && redisConf.Secure && redisConf.CAFile != "" {
			certDir := filepath.Dir(redisConf.CAFile)
			endpoint := fmt.Sprintf("%s/api/worker/config/tls/download", c.ConfigData.End_point)
			if err := c.DownloadAndExtractCerts(endpoint, certDir, c.ConfigData.Credential.Project_id, c.ConfigData.Credential.Secret_key); err != nil {
				return nil, fmt.Errorf("Redis configuration error: failed to download TLS certificates: %w", err)
			}
		}
		return redisConf, nil
	}

	return nil, fmt.Errorf("unsupported broker type %q", brokerType)
}

// GetObject returns the ConfigYamlSupport object.
func (c *ConfigYamlSupport) GetObject() any {
	return c
}

// GetNatsBrokerCon returns the NATS broker connection from the provided interface.
func (c ConfigYamlSupport) GetNatsBrokerCon(gg BrokerConInterface) NatsBrokerConnection {
	kk := gg.GetConnection().(NatsBrokerConnection)
	return kk
}

// GetRabbitMQBrokenCon returns the RabbitMQ broker connection from the provided interface.
func (c ConfigYamlSupport) GetRabbitMQBrokenCon(gg BrokerConInterface) AMQP_BrokerConnection {
	kk := gg.GetConnection().(AMQP_BrokerConnection)
	return kk
}

// GetRedisBrokerCon returns the Redis broker connection from the provided interface.
func (c ConfigYamlSupport) GetRedisBrokerCon(gg BrokerConInterface) RedisBrokerConnection {
	kk := gg.GetConnection().(RedisBrokerConnection)
	return kk
}

// RunChildProcess starts a child process with the current configuration.
// It returns the command object and any error encountered.
func (c *ConfigYamlSupport) RunChildProcess() (*exec.Cmd, error) {
	cmd, err := c.createForChildProcessCommand()
	if err != nil {
		return nil, fmt.Errorf("failed to create command for child process: %w", err)
	}

	// Set environment variables for the command
	cmd.Env = mergeEnv(os.Environ(), c.GetEnv())

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
		Helper.PrintErrName(fmt.Sprintf("Error waiting for command: %v\n", err), "ERR-20230903202")
		return nil, err
	}

	return cmd, nil
}

// RunChildExecsProcess starts a child process for executing commands defined in the configuration.
// It waits for the process to finish and returns any error encountered.
func (c *ConfigYamlSupport) RunChildExecsProcess() (*exec.Cmd, error) {
	cmd, err := c.createForChildExecProcessCommand()
	if err != nil {
		return nil, fmt.Errorf("failed to create command for child exec process: %w", err)
	}
	// Set environment variables for the command
	cmd.Env = mergeEnv(os.Environ(), c.GetEnv())

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
func (c *ConfigYamlSupport) RunExecsProcess(cmd *[]*exec.Cmd) {
	retryCountFirstStart := 5     // Number of retry attempts
	retryDelay := 2 * time.Second // Delay between retries

	for _, execConfig := range c.ConfigData.Execs {
		Helper.PrintGroupName(fmt.Sprintf("Running exec: %s, Key: %s, Cmd: %s\n", execConfig.Name, execConfig.Key, execConfig.Cmd))

		// Resolve working directory
		workingDir := execConfig.Working_dir
		if !filepath.IsAbs(workingDir) {
			workingDir = filepath.Join(filepath.Dir(c.Config_path), workingDir)
		}

		for attempt := 1; attempt <= retryCountFirstStart; attempt++ {

			totalRestartAttempts := execConfig.Attempt
			if totalRestartAttempts <= 0 {
				totalRestartAttempts = 3 // Default to 3 attempts if not specified
			}

			// Create the command
			cmdItem := NewMonitoredCmd(c.createForExecCommand(execConfig, workingDir))
			*cmd = append(*cmd, cmdItem.Cmd)

			// Use the helper function to set up pipes
			stdout, stderr, err := setupCommandPipes(cmdItem.Cmd, execConfig.Name)
			if err != nil {
				*cmd = (*cmd)[:len(*cmd)-1] // Remove the last command if there's an error
				continue
			}

			// Capture and log stdout and stderr
			go printOutputWithIdentity(stdout, execConfig.Name)
			go printOutputWithIdentity(stderr, execConfig.Name)

			// Start the command
			err = cmdItem.Cmd.Start()
			if err != nil {
				Helper.PrintErrName(fmt.Sprintf("Error starting command for %s (Attempt %d/%d): %v\n", execConfig.Name, attempt, retryCountFirstStart, err), "ERR-CMD-START")
				*cmd = (*cmd)[:len(*cmd)-1] // Remove the last command if there's an error
				if attempt == retryCountFirstStart {
					Helper.PrintErrName(fmt.Sprintf("Failed to execute command for %s after %d attempts\n", execConfig.Name, retryCountFirstStart), "ERR-CMD-FAIL")
				}
				if attempt < retryCountFirstStart {
					Helper.PrintGroupName(fmt.Sprintf("Retrying command for %s after %v...\n", execConfig.Name, retryDelay))
					time.Sleep(retryDelay) // Add delay before retrying
				}
				continue
			}

			// Log the running process
			Helper.PrintGroupName(fmt.Sprintf("Command '%s' is running with PID: %d\n", execConfig.Name, cmdItem.Cmd.Process.Pid))

			// Wait for the command and handle restarts
			go func(cmdItem *MonitoredCmd, execName string) {
				for restartAttempt := 0; restartAttempt <= totalRestartAttempts; restartAttempt++ {
					err := cmdItem.Wait()
					if err != nil {
						for in, c := range *cmd {
							if c.Process.Pid == cmdItem.Cmd.Process.Pid {
								*cmd = (*cmd)[:in] // Remove the last command if there's an error
								break
							}
						}

						if restartAttempt == totalRestartAttempts {
							Helper.PrintErrName(fmt.Sprintf("Command '%s' failed after %d attempts. Giving up.\n", execName, totalRestartAttempts), "ERR-2344233432")
							c.ShutdownMainProcess()
							break
						}

						Helper.PrintErrName(fmt.Sprintf("Command '%s' finished with error: %v. Restarting... (Attempt %d/%d)\n", execName, err, restartAttempt+1, totalRestartAttempts), "ERR-234233432")

						if execConfig.Cascade_exit {
							Helper.PrintGroupName(fmt.Sprintf("Cascade exit enabled for command '%s'. Exiting process.\n", execName))
							c.ShutdownMainProcess()
							break
						}

						time.Sleep(2 * time.Second) // Wait before restarting

						cmdItem = NewMonitoredCmd(c.createForExecCommand(execConfig, workingDir))

						stdout, stderr, err := setupCommandPipes(cmdItem.Cmd, execConfig.Name)
						if err != nil {
							*cmd = (*cmd)[:len(*cmd)-1] // Remove the last command if there's an error
							continue
						}

						// Capture and log stdout and stderr
						go printOutputWithIdentity(stdout, execConfig.Name)
						go printOutputWithIdentity(stderr, execConfig.Name)

						cmdItem.Cmd.Start()
						*cmd = append(*cmd, cmdItem.Cmd)
					} else {
						Helper.PrintGroupName(fmt.Sprintf("Command '%s' finished successfully.\n", execName))
						for in, c := range *cmd {
							if c.Process.Pid == cmdItem.Cmd.Process.Pid {
								*cmd = (*cmd)[:in] // Remove the last command if there's an error
								break
							}
						}
						if execConfig.Cascade_exit {
							Helper.PrintGroupName(fmt.Sprintf("Cascade exit enabled for command '%s'. Exiting process.\n", execName))
							c.ShutdownMainProcess()
						}
						break
					}
				}
			}(cmdItem, execConfig.Name)

			break // Exit retry loop on success
		}
	}
}

// GetConfigString reads a string value from a config map without panicking.
// It returns an error when the key is missing, nil, not a string, or empty.
func GetConfigString(config map[string]interface{}, key string) (string, error) {
	if config == nil {
		return "", fmt.Errorf("config is empty, cannot read %q", key)
	}
	raw, ok := config[key]
	if !ok || raw == nil {
		return "", fmt.Errorf("config %q is not specified", key)
	}
	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("config %q has unexpected type %T (expected string)", key, raw)
	}
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("config %q is empty", key)
	}
	return value, nil
}

// GetBrokerConnectionKey returns the broker connection key from the loaded config.
func (c *ConfigYamlSupport) GetBrokerConnectionKey() (string, error) {
	return GetConfigString(c.ConfigData.Broker_connection, "key")
}

func (c *ConfigYamlSupport) ListenForShutdownFromConn(conn BrokerConnectionInterface) {
	if conn == nil {
		Helper.PrintErrName("Skip listening shutdown signal: broker connection is nil", "ERR-SHUTDOWN-LISTEN-NIL")
		return
	}
	identityId := os.Getenv("JOB_ITEM_IDENTITY_ID")
	if identityId == "" {
		identityId = c.ConfigData.Identity_id
	}
	_, err := conn.BasicSub(identityId+".shutdown", func(msg string) {
		fmt.Println("Received shutdown signal from broker connection")
		p, _ := os.FindProcess(os.Getpid())
		p.Signal(syscall.SIGINT)
	})
	if err != nil {
		fmt.Println("Error subscribing to shutdown signal:", err)
		return
	}
}

func (c *ConfigYamlSupport) ShutdownMainProcess() {
	identityId := os.Getenv("JOB_ITEM_IDENTITY_ID")
	fmt.Println("Shutting down main process with identity ID:", identityId)
	brokerKey, err := c.GetBrokerConnectionKey()
	if err != nil {
		Helper.PrintErrName("Cannot shutdown main process: "+err.Error(), "ERR-SHUTDOWN-BROKER-KEY")
		return
	}
	conn := Helper.BrokerConnection.GetConnection(brokerKey)
	if conn == nil {
		fmt.Println("No broker connection available")
		return
	}
	conn.Pub(identityId+".shutdown", "")
}

// monitorCommand listens for when an *exec.Cmd exits, either successfully or with an error.
// It logs the event for each command.
func monitorCommand(cmd *exec.Cmd, name string) {
	go func() {
		err := cmd.Wait()
		if err != nil {
			fmt.Printf("9999999999999999Command '%s' exited with error: %v\n", name, err)
		} else {
			fmt.Printf("8888888888888888Command '%s' exited successfully.\n", name)
		}
	}()
}

// waitWithTimeout waits for the command to finish with a timeout.
// If the timeout is reached, it returns nil without killing the process.
func waitWithTimeout(cmd *exec.Cmd, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		return err // Process finished
	case <-time.After(timeout):
		// Timeout occurred, but do not kill the process
		return nil // Return nil to indicate no error
	}
}

type MonitoredCmd struct {
	Cmd        *exec.Cmd
	waitCalled bool
	mutex      sync.Mutex
}

func (mc *MonitoredCmd) Wait() error {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	if mc.waitCalled {
		return fmt.Errorf("Wait was already called on this command")
	}

	mc.waitCalled = true
	return mc.Cmd.Wait()
}

func NewMonitoredCmd(cmd *exec.Cmd) *MonitoredCmd {
	return &MonitoredCmd{
		Cmd: cmd,
	}
}

// setupCommandPipes sets up stdout and stderr pipes for the given command and returns them.
func setupCommandPipes(cmd *exec.Cmd, identity string) (io.ReadCloser, io.ReadCloser, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		Helper.PrintErrName(fmt.Sprintf("Error creating stdout pipe for %s: %v\n", identity, err), "ERR-STDOUT-PIPE")
		return nil, nil, err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		Helper.PrintErrName(fmt.Sprintf("Error creating stderr pipe for %s: %v\n", identity, err), "ERR-STDERR-PIPE")
		return nil, nil, err
	}

	return stdout, stderr, nil
}
