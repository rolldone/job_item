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

type ConfigData struct {
	End_point  string     `yaml:"end_point"`
	Credential Credential `yaml:"credential"`
	// Import
	Uuid                    string
	Broker_connection       map[string]interface{} `json:"broker_connection,omitempty"`
	Project                 model.ProjectDataView
	Jobs                    []ConfigJob
	Job_item_version_number int    `json:"job_item_version_number,omitempty"`
	Job_item_version        string `json:"job_item_version,omitempty"`
	Job_item_link           string `json:"job_item_link,omitempty"`
}

func ConfigYamlSupportContruct(config_path string) (*ConfigYamlSupport, error) {
	fmt.Println("----------------------------------------------------------")
	stat, err := os.Stat(filepath.Dir(config_path))
	if err != nil {
		fmt.Println("Error working dir :: ", err)
		panic(1)
	}
	if stat.IsDir() {
		fmt.Println("Set working dir :: ", filepath.Dir(config_path))
	} else {
		fmt.Println("Set working dir :: ", ".")
	}
	fmt.Println("----------------------------------------------------------")

	err = os.Chdir(filepath.Dir(config_path))
	if err != nil {
		fmt.Println("Error chdir :: ", err)
		panic(1)
	}

	gg := ConfigYamlSupport{
		Config_path: filepath.Base(config_path),
	}
	gg.loadConfigYaml()
	gg.useEnvToYamlValue()
	err = gg.loadServerCOnfig()
	if err != nil {
		return nil, err
	}
	return &gg, nil
}

type ConfigYamlSupport struct {
	ConfigData        ConfigData
	child_process_app *string
	Config_path       string
}

// Load the config from config.yaml.
// Check it the config problem or not if problem force close.
func (c *ConfigYamlSupport) loadConfigYaml() {
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

func (c *ConfigYamlSupport) useEnvToYamlValue() {
	helper.Fromenv(&c.ConfigData)
}

// Request the config to the server and get configuration.
// Store to the ConfigData Variable.
func (c *ConfigYamlSupport) loadServerCOnfig() error {
	var param = map[string]interface{}{}
	param["project_id"] = c.ConfigData.Credential.Project_id
	param["secret_key"] = c.ConfigData.Credential.Secret_key

	os := runtime.GOOS
	param["os"] = os               // windows | darwin | linux
	param["arch"] = runtime.GOARCH //  386, amd64, arm, s390x

	hostInfo, err := Helper.HardwareInfo.GetInfoHardware()
	if err != nil {
		fmt.Println("err :: ", err)
		panic(1)
	}
	param["host"] = hostInfo
	jsonDataParam, err := json.Marshal(param)
	if err != nil {
		fmt.Println("err :: ", err)
		panic(1)
	}
	var client = &http.Client{}
	request, err := http.NewRequest("POST", fmt.Sprint(c.ConfigData.End_point, "/api/worker/config"), bytes.NewBuffer(jsonDataParam))
	// request.Header.Set("X-Custom-Header", "myvalue")
	request.Header.Set("Content-Type", "application/json")
	if err != nil {
		fmt.Println("err - http.NewRequest :: ", err)
		return err
		// panic(1)
	}
	response, err := client.Do(request)
	if err != nil {
		fmt.Println("err - client.Do :: ", err)
		return err
	}
	defer response.Body.Close()
	bodyData := struct {
		Return ConfigData
	}{}
	err = json.NewDecoder(response.Body).Decode(&bodyData)
	if err != nil {
		fmt.Println("err :: ", err)
		panic(1)
	}
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

// This function for handle when user exit, create other process
// To rename the new app to current app name
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
		fmt.Printf("Received signal: %v\n", sig)
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

	// cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: false}

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

	return cmd, nil
}

func printOutput(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		fmt.Println(strings.TrimSpace(scanner.Text()))
	}
}

func (c *ConfigYamlSupport) GetTypeBrokerCon(v map[string]interface{}) BrokerConInterface {
	switch v["type"].(string) {
	case "nats":
		natsConf := NatsBrokerConnection{}
		natsConf.Host = v["host"].(string)
		natsConf.Key = v["key"].(string)
		natsConf.Name = v["name"].(string)
		natsConf.Port = int(v["port"].(float64))
		natsConf.Type = v["type"].(string)
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
		return redisConf
	}

	return nil
}

func (c *ConfigYamlSupport) GetObject() any {
	return c
}

func (c ConfigYamlSupport) GetNatsBrokerCon(gg BrokerConInterface) NatsBrokerConnection {
	kk := gg.GetConnection().(NatsBrokerConnection)
	return kk
}

func (c ConfigYamlSupport) GetRabbitMQBrokenCon(gg BrokerConInterface) AMQP_BrokerConnection {
	kk := gg.GetConnection().(AMQP_BrokerConnection)
	return kk
}

func (c ConfigYamlSupport) GetRedisBrokerCon(gg BrokerConInterface) RedisBrokerConnection {
	kk := gg.GetConnection().(RedisBrokerConnection)
	return kk
}
