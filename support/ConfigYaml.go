package support

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

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
	Name string `yaml:"name"`
	Key  string `yaml:"key"`
	Cmd  string `yaml:"cmd"`
	// Import
	Pub_type string
}

type ConfigData struct {
	// Local
	Src_path string       `yaml:"src_path"`
	Jobs     *[]ConfigJob `yaml:"jobs" json:"jobs,omitempty"`
	// Import
	Uuid               string
	Broker_connections []map[string]interface{} `yaml:"broker_connections"`
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
	var client = &http.Client{}
	request, err := http.NewRequest("GET", c.ConfigData.Src_path, nil)
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
	c.ConfigData.Uuid = bodyData.Return.Uuid
	c.ConfigData.Broker_connections = bodyData.Return.Broker_connections
	fmt.Println("configData", c.ConfigData)
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
