package support

import (
	"fmt"
	"log"
	"reflect"
)

var Helper *SupportService

func SupportConstruct(segmentApp string) *SupportService {
	gg := SupportService{
		Segment_app: segmentApp,
	}
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
	Segment_app      string
}

func (c *SupportService) PrintGroupName(printText string) {
	fmt.Println(c.Segment_app, " >> ", printText)
}

func (c *SupportService) PrintErrName(printText string) {
	fmt.Println(c.Segment_app, " - ERR >> ", printText)
}

func (c *SupportService) Register(tt SupportInterface) {
	c.PrintGroupName("SupportService - Register :: " + reflect.TypeOf(tt.GetObject()).String())
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
