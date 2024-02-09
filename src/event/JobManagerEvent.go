package event

import (
	"encoding/json"
	"fmt"
	"job_item/support"
	"log"
	"os"
	"os/exec"
	"runtime"

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
	Task_id string       `json:"task_id"`
	Data    DataBodyType `json:"data"`
	Action  string       `json:"action,omitempty"`
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
	conn := support.Helper.BrokerConnection.GetConnection(conn_name)
	c.conn = conn
	project_app_uuid := support.Helper.ConfigYaml.ConfigData.Uuid
	job_datas := support.Helper.ConfigYaml.ConfigData.Project.Job_datas
	jobs := support.Helper.ConfigYaml.ConfigData.Jobs
	for _, v := range jobs {
		jobConfig := v
		isMatch := false
		for _, x := range job_datas {
			if x.Event == v.Event {
				isMatch = true
				// var unsubcribe func()
				sub_key := fmt.Sprint(project_app_uuid, ".", v.Event)
				_, err := conn.Sub(sub_key, project_app_uuid, func(message string) {
					// fmt.Println(sub_key, " :: ", message)
					go func(message string) {
						messageObject := MessageJson{}
						json.Unmarshal([]byte(message), &messageObject)
						if messageObject.Action == "terminate" {
							support.Helper.EventBus.GetBus().Publish(fmt.Sprint(messageObject.Task_id, "_", "terminate"))
						} else {
							dataString, _ := messageObject.Data.ToJSON()
							f, _ := os.Create(fmt.Sprint(messageObject.Task_id, ".json"))
							f.WriteString(dataString)
							f.Close()
							cmd := mustache.Render(jobConfig.Cmd, map[string]string{"task_id": messageObject.Task_id})
							go c.RunGoroutine(cmd, messageObject.Task_id)
						}
					}(message)
				})
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

func (c *JobManagerEvent) RunGoroutine(command string, task_id string) {
	defer func() {
		fmt.Println("Closed goroutine")
		c.conn.Pub(fmt.Sprint(task_id, "_", "finish"), "finish")
	}()
	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", command)
		c.WatchProcessCMD(cmd, task_id)
	} else {
		cmd := exec.Command("bash", "-c", command)
		c.WatchProcessCMD(cmd, task_id)
	}
}

func (c *JobManagerEvent) WatchProcessCMD(cmd *exec.Cmd, task_id string) {

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

	support.Helper.EventBus.GetBus().SubscribeOnce(fmt.Sprint(task_id, "_", "terminate"), func() {
		cmd.Process.Kill()
	})

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
	go func() {
		for {
			n2, err2 := stderr.Read(out)
			if err2 != nil {
				fmt.Println("stderr err :: ", err)
				break
			}
			fmt.Println("stderr :: ", string(out[:n2]))
		}
	}()

	cmd.Wait()
}
