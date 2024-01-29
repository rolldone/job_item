package support

import (
	evbus "github.com/asaskevich/EventBus"
)

func EventBusConstruct() *EventBusSupport {
	vv := EventBusSupport{}
	vv.loadEventBus()
	return &vv
}

type EventBusSupport struct {
	bus evbus.Bus
}

func (c *EventBusSupport) loadEventBus() {
	c.bus = evbus.New()
}

func (c *EventBusSupport) GetObject() any {
	return c
}

func (c *EventBusSupport) GetBus() evbus.Bus {
	return c.bus
}
