package support

import (
	"fmt"

	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
)

func HardwareInfoSupportConstruct() *HardwareInfoSupport {
	gg := HardwareInfoSupport{}
	return &gg
}

type HardwareInfoSupport struct{}

func (c *HardwareInfoSupport) GetInfoHardware() {
	fmt.Println(host.Info())
}

func (c *HardwareInfoSupport) GetInfoNetwork() {

}

func (c *HardwareInfoSupport) GetInfoUsage() {
	v, _ := mem.VirtualMemory()

	// almost every return value is a struct
	fmt.Printf("Total: %v, Free:%v, UsedPercent:%f%%\n", v.Total, v.Free, v.UsedPercent)

	// convert to JSON. String() is also implemented
	fmt.Println(v)
}

func (c *HardwareInfoSupport) GetObject() any {
	return c
}
