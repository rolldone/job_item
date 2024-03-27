package support

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"job_item/support/model"
	"log"
	"net/http"
	"os"
	"os/exec"
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
	// Local
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

func ConfigYamlSupportContruct() *ConfigYamlSupport {
	gg := ConfigYamlSupport{}
	gg.loadConfigYaml()
	gg.loadServerCOnfig()
	return &gg
}

type ConfigYamlSupport struct {
	ConfigData        ConfigData
	child_process_app *string
}

// Load the config from config.yaml.
// Check it the config problem or not if problem force close.
func (c *ConfigYamlSupport) loadConfigYaml() {
	yamlFile, err := os.ReadFile("config.yaml")
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

// Request the config to the server and get configuration.
// Store to the ConfigData Variable.
func (c *ConfigYamlSupport) loadServerCOnfig() {
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
	request, err := http.NewRequest("POST", c.ConfigData.End_point, bytes.NewBuffer(jsonDataParam))
	// request.Header.Set("X-Custom-Header", "myvalue")
	request.Header.Set("Content-Type", "application/json")
	if err != nil {
		fmt.Println("err :: ", err)
		panic(1)
	}
	response, err := client.Do(request)
	if err != nil {
		fmt.Println("err :: ", err)
		panic(1)
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
}

func (c *ConfigYamlSupport) DownloadNewApp() error {
	url := c.ConfigData.Job_item_link
	outputPath := ""
	os_type := runtime.GOOS
	switch os_type {
	case "windows":
		outputPath = "job_item.exe"
	case "darwin":
		outputPath = "job_item"
	case "linux":
		outputPath = "job_item"
	}
	c.child_process_app = &outputPath
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
	switch os_type {
	case "windows":
		cmd = exec.Command(executablePath, "child_process")
	case "darwin":
		cmd = exec.Command(executablePath, "child_process")
	case "linux":
		cmd = exec.Command(executablePath, "child_process")
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: false}

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
