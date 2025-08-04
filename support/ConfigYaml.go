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
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"job_item/src/helper"
	"job_item/support/model"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
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
	Name        string            `yaml:"name"`
	Key         string            `yaml:"key"`
	Cmd         string            `yaml:"cmd"`
	Env         map[string]string `yaml:"env"`
	Working_dir string            `yaml:"working_dir"` // Add Working_dir field
}

type ConfigData struct {
	End_point  string     `yaml:"end_point"`
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
	// LoadConfigYaml loads the configuration from a YAML file.
	RequestToServer bool
	Config_path     string
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
	if props.RequestToServer {
		if gg.ConfigData.End_point != "" {
			err = gg.loadServerCOnfig()
			if err != nil {
				return nil, err
			}
		} else {
			fmt.Println("WARNING: End point is not set, using local config only")
		}
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
	err = yaml.Unmarshal(yamlFile, &c.ConfigData)
	if err != nil {
		log.Fatalf("Unmarshal: %v", err)
	}
}

// useEnvToYamlValue updates the configuration values using environment variables.
func (c *ConfigYamlSupport) useEnvToYamlValue() {
	helper.Fromenv(&c.ConfigData)
}

// loadServerCOnfig requests the configuration from the server and updates the ConfigData.
// It returns an error if the request fails or the response is invalid.
func (c *ConfigYamlSupport) loadServerCOnfig() error {
	var param = map[string]interface{}{}
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
		fmt.Printf("[%s] %s\n", identity, strings.TrimSpace(scanner.Text()))
	}
}

// GetTypeBrokerCon returns the appropriate broker connection type based on the provided configuration.
func (c *ConfigYamlSupport) GetTypeBrokerCon(v map[string]interface{}) BrokerConInterface {
	switch v["type"].(string) {
	case "nats":
		natsConf := NatsBrokerConnection{}
		natsConf.Host = v["host"].(string)
		natsConf.Key = v["key"].(string)
		natsConf.Name = v["name"].(string)
		natsConf.Port = int(v["port"].(float64))
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
		return natsConf
	case "rabbitmq":
		rabbitmqConf := AMQP_BrokerConnection{}
		rabbitmqConf.Host = v["host"].(string)
		rabbitmqConf.Key = v["key"].(string)
		rabbitmqConf.Name = v["name"].(string)
		rabbitmqConf.Port = int(v["port"].(float64))
		rabbitmqConf.Type = v["type"].(string)
		rabbitmqConf.User = v["user"].(string)
		rabbitmqConf.Password = v["password"].(string)
		return rabbitmqConf
	case "redis":
		redisConf := RedisBrokerConnection{}
		redisConf.Host = v["host"].(string)
		redisConf.Key = v["key"].(string)
		redisConf.Name = v["name"].(string)
		redisConf.Port = int(v["port"].(float64))
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
