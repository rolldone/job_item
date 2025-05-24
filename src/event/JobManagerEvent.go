package event

import (
	"encoding/json"
	"fmt"
	"job_item/support"
	"log"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/hoisie/mustache"
)

type DataBodyType map[string]interface{}

func (c *DataBodyType) ToJSON() (string, error) {
	gg, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return string(gg), nil
}

type MessageJson struct {
	Task_id string      `json:"task_id"`
	Data    interface{} `json:"data"`
	Action  string      `json:"action,omitempty"`
}

func JobManagerEventConstruct() JobManagerEvent {
	gg := JobManagerEvent{}
	return gg
}

type JobManagerEvent struct {
	conn support.BrokerConnectionInterface
}

const (
	START     = 1
	TERMINATE = 2
)

type ActionMap struct {
	Action string
}

func (c *JobManagerEvent) ListenEvent(conn_name string) {
	// Then get the data connection by connection key
	var initPubSubChannel = func() {
		conn := support.Helper.BrokerConnection.GetConnection(conn_name)
		c.conn = conn
		project_app_uuid := support.Helper.ConfigYaml.ConfigData.Uuid
		job_datas := support.Helper.ConfigYaml.ConfigData.Project.Job_datas
		jobs := support.Helper.ConfigYaml.ConfigData.Jobs
		unsubcribes := []func(){}
		for _, v := range jobs {
			jobConfig := v
			isMatch := false
			for _, x := range job_datas {
				if x.Event == v.Event {
					isMatch = true
					// var unsubcribe func()
					sub_key := fmt.Sprint(project_app_uuid, ".", v.Event)
					unsub, err := conn.Sub(sub_key, project_app_uuid, func(message string) {
						// fmt.Println(sub_key, " :: ", message)
						go func(message string) {
							messageObject := MessageJson{}
							json.Unmarshal([]byte(message), &messageObject)
							dataString, _ := json.Marshal(messageObject.Data) // .Data.ToJSON()
							f, _ := os.Create(fmt.Sprint(messageObject.Task_id, ".json"))
							f.WriteString(string(dataString))
							f.Close()
							// messageObject.Data["task_id"] = messageObject.Task_id
							var cmd string
							if _, ok := messageObject.Data.([]interface{}); ok {
								cmd = mustache.Render(jobConfig.Cmd, map[string]string{"task_id": messageObject.Task_id})
							} else {
								var messageObjectParse map[string]string
								// Marshal the interface to JSON
								jsonData, err := json.Marshal(messageObject.Data)
								if err != nil {
									fmt.Println("Error marshaling interface to JSON:", err)
									return
								}
								json.Unmarshal([]byte(jsonData), &messageObjectParse)
								messageObjectParse["task_id"] = messageObject.Task_id
								cmd = mustache.Render(jobConfig.Cmd, messageObjectParse)
							}

							jobManEvItem := JobManagerEventItem{
								conn: c.conn,
							}
							go jobManEvItem.RunGoroutine(cmd, messageObject.Task_id)
						}(message)
					})

					_ = append(unsubcribes, unsub)

					if err != nil {
						log.Println(err)
						break
					}
					break
				}
			}
			if !isMatch {
				fmt.Println("Event :", v.Event, " not register yet. Please check on job manager with app project that you register it.")
			}
		}
	}
	initPubSubChannel()
	// Subscribe to a custom event (replace "custom_event" and handler as needed)
	support.Helper.EventBus.GetBus().Subscribe(c.conn.GetRefreshPubSub(), func(data interface{}) {
		fmt.Println("Received custom_event with data:", data)
		initPubSubChannel()
	})
}

type JobManagerEventItem struct {
	conn        support.BrokerConnectionInterface
	Last_status string
}

func (c *JobManagerEventItem) RunGoroutine(command string, task_id string) {
	project_app_uuid := support.Helper.ConfigYaml.ConfigData.Uuid
	unsub, err := c.conn.Sub(task_id+"_worker", project_app_uuid, func(message string) {
		// fmt.Println(sub_key, " :: ", message)
		messageObject := MessageJson{}
		json.Unmarshal([]byte(message), &messageObject)
		if messageObject.Action == GetStatus().STATUS_TIMEOUT {
			c.Last_status = GetStatus().STATUS_TIMEOUT
			support.Helper.EventBus.GetBus().Publish(fmt.Sprint(messageObject.Task_id, "_", "timeout"))
		} else if messageObject.Action == GetStatus().STATUS_TERMINATE {
			c.Last_status = GetStatus().STATUS_TERMINATE
			support.Helper.EventBus.GetBus().Publish(fmt.Sprint(messageObject.Task_id, "_", "terminate"))
		}
	})
	if err != nil {
		log.Println("RunGoroutine :: err :: 23940239409 :: ", err)
	}

	c.Last_status = GetStatus().STATUS_FINISH
	defer func(last_status *string) {
		unsub()
		fmt.Println("Closed goroutine")
		time.Sleep(time.Duration(time.Second) * 3)
		c.conn.Pub(fmt.Sprint(task_id, "_", "finish"), *last_status)
	}(&c.Last_status)

	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", "/K", command)
		c.WatchProcessCMD(cmd, task_id)
	} else {
		cmd := exec.Command("bash", "-c", command)
		c.WatchProcessCMD(cmd, task_id)
	}

}

func (c *JobManagerEventItem) WatchProcessCMD(cmd *exec.Cmd, task_id string) {

	// Create function kill process
	killProcess := func() {
		cmd.Process.Kill()
	}

	// Subcribe the kill process action
	support.Helper.EventBus.GetBus().SubscribeOnce(fmt.Sprint(task_id, "_", "timeout"), killProcess)
	support.Helper.EventBus.GetBus().SubscribeOnce(fmt.Sprint(task_id, "_", "terminate"), killProcess)

	// If get defer unsbcribe the event bus
	defer func() {
		support.Helper.EventBus.GetBus().Unsubscribe(fmt.Sprint(task_id, "_", "timeout"), killProcess)
		support.Helper.EventBus.GetBus().Unsubscribe(fmt.Sprint(task_id, "_", "terminate"), killProcess)
		fmt.Println("Close the subcribe listen timeout and terminate")
	}()

	// creating a std pipeline
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Println(err)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		fmt.Println(err)
		return
	}

	// starting the command
	err = cmd.Start()

	if err != nil {
		return
	}

	out := make([]byte, 1024)

	go func(conn support.BrokerConnectionInterface) {
		for {
			n2, err2 := stderr.Read(out)
			if err2 != nil {
				fmt.Println("stderr err :: ", err)
				break
			}
			fmt.Println("stderr :: ", string(out[:n2]))
			conn.Pub(task_id+"_failed", string(out[:n2]))
			c.Last_status = GetStatus().STATUS_ERROR
		}
	}(c.conn)

	go func(conn support.BrokerConnectionInterface) {
		for {
			// reading the bytes
			n, err := stdout.Read(out)
			if err != nil {
				fmt.Println("stdout err :: ", err)
				break
			}
			fmt.Println("stdout :: ", string(out[:n]))
			// conn.Pub(task_id+"_process", fmt.Sprint("stdout :: ", string(out[:n])))
			conn.Pub(task_id+"_process", string(out[:n]))
		}
	}(c.conn)

	cmd.Wait()
}

type brokerConStatus struct {
	STATUS_ERROR     string
	STATUS_TIMEOUT   string
	STATUS_TERMINATE string
	STATUS_FINISH    string
}

// Create get status follow by Type jobRecordStatus.
// Define the value by each value.
// Return in as type jobRecordStatus
func GetStatus() brokerConStatus {
	return brokerConStatus{
		STATUS_ERROR:     "error",
		STATUS_TIMEOUT:   "timeout",
		STATUS_TERMINATE: "terminate",
		STATUS_FINISH:    "finish",
	}
}
