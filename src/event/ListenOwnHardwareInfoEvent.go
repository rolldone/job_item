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

type AddPayload_MemOwnHardware struct {
	Host_id           string                 `json:"host_id"`
	Data              *mem.VirtualMemoryStat `json:"data"`
	Time_duration     string                 `json:"time_duration"`
	Project_data_uuid string                 `json:"project_data_uuid"`
}

type AddPayload_CpuOwnHardware struct {
	Host_id           string    `json:"host_id"`
	Data              []float64 `json:"data"`
	Time_duration     string    `json:"time_duration"`
	Project_data_uuid string    `json:"project_data_uuid"`
}

type AddPayload_NetOwnHardware struct {
	Host_id           string                `json:"host_id"`
	Data              net.InterfaceStatList `json:"data"`
	Time_duration     string                `json:"time_duration"`
	Project_data_uuid string                `json:"project_data_uuid"`
}

const (
	ONE_MINUTE  = "one_minute"  // time.Duration(time.Minute * 1)
	FIVE_MINUTE = "five_minute" // time.Duration(time.Minute * 5)
	FIVE_SECOND = "five_second" // time.Duration(time.Second * 5)
	TEN_SECOND  = "ten_second"  // time.Duration(time.Second * 10)
	SECOND_30   = "30_second"   // time.Duration(time.Second * 30)
)

func (c *ListenOwnHardwareInfoEvent) GetDuration(time_duration string) time.Duration {
	switch time_duration {
	case FIVE_MINUTE:
		return time.Duration(time.Minute * 5)
	case FIVE_SECOND:
		return time.Duration(time.Second * 5)
	case TEN_SECOND:
		return time.Duration(time.Second * 10)
	case SECOND_30:
		return time.Duration(time.Second * 30)
	case ONE_MINUTE:
		return time.Duration(time.Minute * 1)
	default:
		return time.Duration(time.Minute * 5)
	}
}

func (c *ListenOwnHardwareInfoEvent) ListenInfoHardware(conn_name string) {
	conn := support.Helper.BrokerConnection.GetConnection(conn_name)
	// Subscribe to refresh_pubsub event to re-init conn
	support.Helper.EventBus.GetBus().Subscribe(conn.GetRefreshPubSub(), func(data interface{}) {
		conn = support.Helper.BrokerConnection.GetConnection(conn_name)
		log.Println("[ListenOwnHardwareInfoEvent] Refreshed pubsub connection for:", conn_name)
	})

	go func() {
		for {
			if !(conn).IsConnected() {
				log.Println("Connection is not active, skipping publish for ListenInfoHardware")
				time.Sleep(c.GetDuration(FIVE_MINUTE))
				continue
			}

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

			conn.Pub("listen_host_information", string(hostInfoJsonString))

			time.Sleep(c.GetDuration(FIVE_MINUTE))
		}
	}()
}

func (c *ListenOwnHardwareInfoEvent) ListenInfoUsage(conn_name string) {
	conn := support.Helper.BrokerConnection.GetConnection(conn_name)
	// Subscribe to refresh_pubsub event to re-init conn
	support.Helper.EventBus.GetBus().Subscribe(conn.GetRefreshPubSub(), func(data interface{}) {
		conn = support.Helper.BrokerConnection.GetConnection(conn_name)
		log.Println("[ListenOwnHardwareInfoEvent] Refreshed pubsub connection for:", conn_name)
	})

	go func() {
		for {
			if !conn.IsConnected() {
				log.Println("Connection is not active, skipping publish for ListenInfoUsage")
				time.Sleep(c.GetDuration(ONE_MINUTE))
				continue
			}

			hostInfo, err := host.Info()
			if err != nil {
				log.Fatalln(err)
				return
			}

			p, err := cpu.Percent(time.Duration(time.Second*1), true)
			if err != nil {
				log.Fatalln(err)
				break
			}

			cp := AddPayload_CpuOwnHardware{
				Host_id:           hostInfo.HostID,
				Data:              p,
				Time_duration:     ONE_MINUTE,
				Project_data_uuid: support.Helper.ConfigYaml.ConfigData.Uuid,
			}

			pString, err := json.Marshal(cp)
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

			mp := AddPayload_MemOwnHardware{
				Host_id:           hostInfo.HostID,
				Data:              v,
				Time_duration:     ONE_MINUTE,
				Project_data_uuid: support.Helper.ConfigYaml.ConfigData.Uuid,
			}

			vString, err := json.Marshal(mp)
			if err != nil {
				log.Fatalln(err)
				return
			}

			conn.Pub("listen_mem_information", string(vString))

			time.Sleep(c.GetDuration(ONE_MINUTE))
		}
	}()
}

func (c *ListenOwnHardwareInfoEvent) ListenInfoNetwork(conn_name string) {
	conn := support.Helper.BrokerConnection.GetConnection(conn_name)
	// Subscribe to refresh_pubsub event to re-init conn
	support.Helper.EventBus.GetBus().Subscribe(conn.GetRefreshPubSub(), func(data interface{}) {
		conn = support.Helper.BrokerConnection.GetConnection(conn_name)
		log.Println("[ListenOwnHardwareInfoEvent] Refreshed pubsub connection for:", conn_name)
	})

	go func() {
		for {
			if !conn.IsConnected() {
				log.Println("Connection is not active, skipping publish for ListenInfoNetwork")
				time.Sleep(c.GetDuration(ONE_MINUTE))
				continue
			}

			hostInfo, err := host.Info()
			if err != nil {
				log.Fatalln(err)
				return
			}

			netInterfaces, err := net.Interfaces()
			if err != nil {
				log.Fatalln(err)
				return
			}

			cp := AddPayload_NetOwnHardware{
				Host_id:           hostInfo.HostID,
				Data:              netInterfaces,
				Time_duration:     ONE_MINUTE,
				Project_data_uuid: support.Helper.ConfigYaml.ConfigData.Uuid,
			}

			netInterfaceString, err := json.Marshal(cp)
			if err != nil {
				log.Fatalln(err)
				return
			}

			conn.Pub("listen_net_information", string(netInterfaceString))

			time.Sleep(c.GetDuration(ONE_MINUTE))
		}
	}()
}
