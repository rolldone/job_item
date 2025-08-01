package support

import (
	"fmt"
	"log"
	"reflect"
)

var Helper *SupportService

func SupportConstruct() *SupportService {
	gg := SupportService{}
	Helper = &gg
	return &gg
}

type SupportInterface interface {
	GetObject() any
}

type SupportService struct {
	ConfigYaml       *ConfigYamlSupport
	BrokerConnection *BrokerConnectionSupport
	EventBus         *EventBusSupport
	HardwareInfo     *HardwareInfoSupport
	Gin              *GinSupport
}

func (c *SupportService) Register(tt SupportInterface) {
	fmt.Println("SupportService - Register :: ", reflect.TypeOf(tt.GetObject()))
	switch tt.GetObject().(type) {
	case *ConfigYamlSupport:
		c.ConfigYaml = tt.(*ConfigYamlSupport)
	case *BrokerConnectionSupport:
		c.BrokerConnection = tt.(*BrokerConnectionSupport)
	case *EventBusSupport:
		c.EventBus = tt.(*EventBusSupport)
	case *HardwareInfoSupport:
		c.HardwareInfo = tt.(*HardwareInfoSupport)
	case *GinSupport:
		c.Gin = tt.(*GinSupport)
	default:
		log.Fatal("This struct is part of interface but not register yet to SupportService. Please Register it")
		panic(1)
	}
}
