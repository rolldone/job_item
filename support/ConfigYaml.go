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
	"encoding/json"
	"fmt"
	"io"
	"job_item/src/helper"
	support_helper "job_item/support/helper"
	"job_item/support/model"
	"log"
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

	gg := ConfigYamlSupport{
		Config_path: filepath.Base(props.Config_path),
	}

	stat, err := os.Stat(filepath.Dir(props.Config_path))
	if err != nil {
		gg.printGroupName("Error getting directory stat: " + err.Error())
		panic(1)
	}
	if stat.IsDir() {
		gg.printGroupName("Set working dir :: " + filepath.Dir(props.Config_path))
	} else {
		gg.printGroupName("Set working dir :: .")
	}

	err = os.Chdir(filepath.Dir(props.Config_path))
	if err != nil {
		gg.printGroupName("Error chdir :: " + err.Error())
		panic(1)
	}

	gg.LoadConfigYaml()
	gg.useEnvToYamlValue()
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
	return &gg, nil
}

func (c *ConfigYamlSupport) printGroupName(printText string) {
	fmt.Println(Helper.Segment_app, " >> ", printText)
}

type ConfigYamlSupport struct {
	ConfigData        ConfigData
	child_process_app *string
	Config_path       string
}

// LoadConfigYaml loads the configuration from a YAML file.
// It panics if the file cannot be read or parsed.
func (c *ConfigYamlSupport) LoadConfigYaml() {
	yamlFile, err := os.ReadFile(c.Config_path)
	if err != nil {
		fmt.Println("Problem open file config.yaml ", err)
		panic(1)
	}
	c.ConfigData = ConfigData{}

	// Generate a new UUID for the Identity_id
	identityId := os.Getenv("JOB_ITEM_IDENTITY_ID")
	if identityId == "" {
		idenityIdNew, err := support_helper.GenerateUUIDv7()
		if err != nil {
			fmt.Println("Failed to generate UUID:", err)
			panic(1)
		}
		identityId = idenityIdNew
	}
	c.ConfigData.Identity_id = identityId

	err = yaml.Unmarshal(yamlFile, &c.ConfigData)
	if err != nil {
		log.Fatalf("Unmarshal: %v", err)
	}
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
		// Specific endpoints for child process to add/get share data
		"JOB_ITEM_SHARE_DATA_HOST=" + shareHost,
		"JOB_ITEM_SHARE_DATA_ADD=" + jobItemShareDataAdd,
		"JOB_ITEM_SHARE_DATA_GET=" + jobItemShareDataGet,
	}
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
// It returns an error if the request fails or the response is invalid.
func (c *ConfigYamlSupport) loadServerCOnfig() error {
	var param = map[string]interface{}{}

	if os.Getenv("JOB_ITEM_CONFIG_DATA") != "" {
		// If JOB_ITEM_CONFIG_DATA is set, use it as the config data
		configDataString := os.Getenv("JOB_ITEM_CONFIG_DATA")
		var configData ConfigData
		err := json.Unmarshal([]byte(configDataString), &configData)
		if err != nil {
			c.printGroupName("Error unmarshalling JOB_ITEM_CONFIG_DATA: " + err.Error())
			panic(1)
		}
		c.ConfigData = configData
		c.printGroupName("Using JOB_ITEM_CONFIG_DATA from environment variable")
		return nil
	}

	param["project_id"] = c.ConfigData.Credential.Project_id
	param["secret_key"] = c.ConfigData.Credential.Secret_key

	os := runtime.GOOS
	param["os"] = os               // windows | darwin | linux
	param["arch"] = runtime.GOARCH //  386, amd64, arm, s390x

	hostInfo, err := Helper.HardwareInfo.GetInfoHardware()
	if err != nil {
		c.printGroupName("Error getting hardware info: " + err.Error())
		panic(1)
	}
	param["host"] = hostInfo
	jsonDataParam, err := json.Marshal(param)
	if err != nil {
		c.printGroupName("Error marshalling JSON: " + err.Error())
		panic(1)
	}
	var client = &http.Client{}
	endpoint := fmt.Sprint(c.ConfigData.End_point, "/api/worker/config")
	c.printGroupName("Attempting to connect to server: " + endpoint)

	request, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonDataParam))
	// request.Header.Set("X-Custom-Header", "myvalue")
	request.Header.Set("Content-Type", "application/json")
	if err != nil {
		c.printGroupName("Failed to create HTTP request to " + endpoint + " :: " + err.Error())
		return err
		// panic(1)
	}

	c.printGroupName("Sending configuration request to server...")
	response, err := client.Do(request)
	if err != nil {
		c.printGroupName("Connection failed to server " + endpoint)
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
		c.printGroupName("ERROR: Server returned HTTP " + fmt.Sprint(response.StatusCode) + " status code")
		c.printGroupName("Expected 200 OK but got " + response.Status)
		return fmt.Errorf("server returned non-200 status: %d %s", response.StatusCode, response.Status)
	}

	c.printGroupName("Successfully connected to server (HTTP " + fmt.Sprint(response.StatusCode) + ")")
	bodyData := struct {
		Return ConfigData
	}{}
	err = json.NewDecoder(response.Body).Decode(&bodyData)
	if err != nil {
		c.printGroupName("ERROR: Failed to parse server response JSON :: " + err.Error())
		c.printGroupName("This could mean the server returned invalid JSON format")
		panic(1)
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

// GetTypeBrokerCon returns the appropriate broker connection type based on the provided configuration.
func (c *ConfigYamlSupport) GetTypeBrokerCon(v map[string]interface{}) BrokerConInterface {
	switch v["type"].(string) {
	case "nats":
		natsConf := NatsBrokerConnection{}
		natsConf.Host = v["host"].(string)
		natsConf.Key = v["key"].(string)
		natsConf.Name = v["name"].(string)
		switch port := v["port"].(type) {
		case int:
			natsConf.Port = port
		case float64:
			natsConf.Port = int(port)
		default:
			panic(fmt.Sprintf("unexpected type for port: %T", port))
		}
		natsConf.Type = v["type"].(string)
		if v["auth_type"] != nil {
			natsConf.Auth_type = v["auth_type"].(string)
		} else {
			fmt.Println("NATS ERR : You need define auth_type : [none | user_password | token], on job_manager")
			panic(1)
		}
		if v["user"] != nil {
			natsConf.User = v["user"].(string)
		}
		if v["password"] != nil {
			natsConf.Password = v["password"].(string)
		}
		if v["token"] != nil {
			natsConf.Token = v["token"].(string)
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
		if v["ca_file"] != nil {
			natsConf.CAFile = v["ca_file"].(string)
		}
		if v["cert_file"] != nil {
			natsConf.CertFile = v["cert_file"].(string)
		}
		if v["key_file"] != nil {
			natsConf.KeyFile = v["key_file"].(string)
		}
		if Helper.ConfigYaml.ConfigData.End_point != "" && natsConf.Secure && natsConf.CAFile != "" {
			certDir := filepath.Dir(natsConf.CAFile)
			endpoint := fmt.Sprintf("%s/api/worker/config/tls/download", c.ConfigData.End_point)
			if err := c.DownloadAndExtractCerts(endpoint, certDir, c.ConfigData.Credential.Project_id, c.ConfigData.Credential.Secret_key); err != nil {
				panic(err)
			}
		}
		return natsConf
	case "rabbitmq":
		rabbitmqConf := AMQP_BrokerConnection{}
		rabbitmqConf.Host = v["host"].(string)
		rabbitmqConf.Key = v["key"].(string)
		rabbitmqConf.Name = v["name"].(string)
		switch port := v["port"].(type) {
		case int:
			rabbitmqConf.Port = port
		case float64:
			rabbitmqConf.Port = int(port)
		default:
			panic(fmt.Sprintf("unexpected type for port: %T", port))
		}
		rabbitmqConf.Type = v["type"].(string)
		rabbitmqConf.User = v["user"].(string)
		rabbitmqConf.Password = v["password"].(string)
		if v["exchange"] != nil {
			rabbitmqConf.Exchange = v["exchange"].(string)
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
		if v["ca_file"] != nil {
			rabbitmqConf.CAFile = v["ca_file"].(string)
		}
		if v["cert_file"] != nil {
			rabbitmqConf.CertFile = v["cert_file"].(string)
		}
		if v["key_file"] != nil {
			rabbitmqConf.KeyFile = v["key_file"].(string)
		}
		if Helper.ConfigYaml.ConfigData.End_point != "" && rabbitmqConf.Secure && rabbitmqConf.CAFile != "" {
			certDir := filepath.Dir(rabbitmqConf.CAFile)
			endpoint := fmt.Sprintf("%s/api/worker/config/tls/download", c.ConfigData.End_point)
			if err := c.DownloadAndExtractCerts(endpoint, certDir, c.ConfigData.Credential.Project_id, c.ConfigData.Credential.Secret_key); err != nil {
				panic(err)
			}
		}
		return rabbitmqConf
	case "redis":
		redisConf := RedisBrokerConnection{}
		redisConf.Host = v["host"].(string)
		redisConf.Key = v["key"].(string)
		redisConf.Name = v["name"].(string)
		switch port := v["port"].(type) {
		case int:
			redisConf.Port = port
		case float64:
			redisConf.Port = int(port)
		default:
			panic(fmt.Sprintf("unexpected type for port: %T", port))
		}
		redisConf.Type = v["type"].(string)
		if v["password"] != nil {
			redisConf.Password = v["password"].(string)
		} else {
			fmt.Println("REDIS ERR : You need define password")
			panic(1)
		}
		if v["db"] != nil {
			redisConf.Db = int(v["db"].(float64))
		} else {
			fmt.Println("REDIS ERR : You need define db")
			panic(1)
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
		if v["ca_file"] != nil {
			redisConf.CAFile = v["ca_file"].(string)
		}
		if v["cert_file"] != nil {
			redisConf.CertFile = v["cert_file"].(string)
		}
		if v["key_file"] != nil {
			redisConf.KeyFile = v["key_file"].(string)
		}
		if Helper.ConfigYaml.ConfigData.End_point != "" && redisConf.Secure && redisConf.CAFile != "" {
			certDir := filepath.Dir(redisConf.CAFile)
			endpoint := fmt.Sprintf("%s/api/worker/config/tls/download", c.ConfigData.End_point)
			if err := c.DownloadAndExtractCerts(endpoint, certDir, c.ConfigData.Credential.Project_id, c.ConfigData.Credential.Secret_key); err != nil {
				panic(err)
			}
		}
		return redisConf
	}

	return nil
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
	cmd.Env = append(os.Environ(), c.GetEnv()...)

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
	cmd.Env = append(os.Environ(), c.GetEnv()...)

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

func (c *ConfigYamlSupport) ListenForShutdownFromConn(conn BrokerConnectionInterface) {
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
	conn := Helper.BrokerConnection.GetConnection(c.ConfigData.Broker_connection["key"].(string))
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
