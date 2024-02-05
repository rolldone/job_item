package event

import (
	"encoding/json"
	"job_item/support"
	"log"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

type ListenOwnHardwareInfoEvent struct{}

func (c *ListenOwnHardwareInfoEvent) ListenInfoHardware(conn_name string) {
	conn := support.Helper.BrokerConnection.GetConnection(conn_name)

	go func() {
		for {
			hostInfo, err := host.Info()
			if err != nil {
				log.Fatalln(err)
				return
			}
			hostInfoJsonString, err := json.Marshal(hostInfo)
			if err != nil {
				log.Fatalln(err)
				return
			}
			time.Sleep(time.Duration(time.Minute * 5))
			conn.Pub("listen_host_information", string(hostInfoJsonString))
			// fmt.Println(host.Info())
		}
	}()

}

func (c *ListenOwnHardwareInfoEvent) ListenInfoNetwork(conn_name string) {
	conn := support.Helper.BrokerConnection.GetConnection(conn_name)
	go func() {
		for {
			time.Sleep(time.Duration(time.Second * 5))
			netInterfaces, err := net.Interfaces()
			if err != nil {
				log.Fatalln(err)
				return
			}
			netInterfaceString, err := json.Marshal(netInterfaces)
			if err != nil {
				log.Fatalln(err)
				return
			}
			conn.Pub("listen_net_information", string(netInterfaceString))
			// fmt.Println(net.Interfaces())
		}
	}()

}

func (c *ListenOwnHardwareInfoEvent) ListenInfoUsage(conn_name string) {
	conn := support.Helper.BrokerConnection.GetConnection(conn_name)

	go func() {
		for {
			time.Sleep(time.Duration(time.Second * 5))
			p, err := cpu.Percent(time.Duration(time.Second*1), true)
			if err != nil {
				log.Fatalln(err)
				break
			}
			// fmt.Println("CPU :: ", p)
			pString, err := json.Marshal(p)
			if err != nil {
				log.Fatalln(err)
				return
			}
			conn.Pub("listen_cpu_information", string(pString))

			v, err := mem.VirtualMemory()
			if err != nil {
				log.Fatalln(err)
				break
			}
			// fmt.Println("MEM :: ", v)
			vString, err := json.Marshal(v)
			if err != nil {
				log.Fatalln(err)
				return
			}
			conn.Pub("listen_mem_information", string(vString))
		}
	}()

	// // almost every return value is a struct
	// fmt.Printf("Total: %v, Free:%v, UsedPercent:%f%%\n", v.Total, v.Free, v.UsedPercent)

	// // convert to JSON. String() is also implemented

}
