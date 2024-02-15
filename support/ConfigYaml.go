package support

import (
	"bytes"
	"encoding/json"
	"fmt"
	"job_item/support/model"
	"log"
	"net/http"
	"os"

	"dario.cat/mergo"
	"gopkg.in/yaml.v3"
)

type NatsBrokerConnection struct {
	Name string `yaml:"name"`
	Key  string `yaml:"key"`
	Type string `yaml:"type"`
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

func (c NatsBrokerConnection) GetConnection() any {
	return c
}

type RabbitMQ_BrokerConnection struct {
	Name string `yaml:"name"`
	Key  string `yaml:"key"`
	Type string `yaml:"type"`
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

func (c RabbitMQ_BrokerConnection) GetConnection() any {
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
	Uuid              string
	Broker_connection map[string]interface{}
	Project           model.ProjectDataView
	Jobs              []ConfigJob
}

func ConfigYamlSupportContruct() *ConfigYamlSupport {
	gg := ConfigYamlSupport{}
	gg.loadConfigYaml()
	gg.loadServerCOnfig()
	return &gg
}

type ConfigYamlSupport struct {
	ConfigData ConfigData
}

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

func (c *ConfigYamlSupport) loadServerCOnfig() {
	var param = map[string]interface{}{}
	param["project_id"] = c.ConfigData.Credential.Project_id
	param["secret_key"] = c.ConfigData.Credential.Secret_key
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
	mergo.Merge(&c.ConfigData, bodyData.Return)
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
		return natsConf
	case "rabbitmq":
		rabbitmqConf := RabbitMQ_BrokerConnection{}
		rabbitmqConf.Host = v["host"].(string)
		rabbitmqConf.Key = v["key"].(string)
		rabbitmqConf.Name = v["name"].(string)
		rabbitmqConf.Port = int(v["port"].(int))
		rabbitmqConf.Type = v["type"].(string)
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

func (c ConfigYamlSupport) GetRabbitMQBrokenCon(gg BrokerConInterface) RabbitMQ_BrokerConnection {
	kk := gg.GetConnection().(RabbitMQ_BrokerConnection)
	return kk
}
