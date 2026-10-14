package event

import (
	"encoding/json"
	"fmt"
	"job_item/src/helper"
	"job_item/support"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
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

func JobManagerEventConstruct() *JobManagerEvent {
	return &JobManagerEvent{}
}

type JobManagerEvent struct {
	conn           support.BrokerConnectionInterface
	controlUnsubMu sync.Mutex
	controlUnsub   func()
}

// Helper function to subscribe and process job events
func subscribeAndRunJobEvent(conn support.BrokerConnectionInterface, sub_key string, template string, project_app_uuid string, c *JobManagerEvent) (func(), error) {
	unsub, err := conn.Sub(sub_key, project_app_uuid, func(message string) {
		go func(message string) {
			messageObject := MessageJson{}
			json.Unmarshal([]byte(message), &messageObject)
			dataString, _ := json.Marshal(messageObject.Data)
			f, _ := os.Create(fmt.Sprint(messageObject.Task_id, ".json"))
			f.WriteString(string(dataString))
			f.Close()

			conn.Pub(messageObject.Task_id+"_who", support.Helper.ConfigYaml.ConfigData.Identity_id)

			var cmd string
			if _, ok := messageObject.Data.([]interface{}); ok {
				cmd = mustache.Render(template, map[string]string{"task_id": messageObject.Task_id})
			} else {
				var messageObjectParse map[string]string
				jsonData, err := json.Marshal(messageObject.Data)
				if err != nil {
					fmt.Println("Error marshaling interface to JSON:", err)
					return
				}
				json.Unmarshal([]byte(jsonData), &messageObjectParse)
				messageObjectParse["task_id"] = messageObject.Task_id
				cmd = mustache.Render(template, messageObjectParse)
			}

			jobManEvItem := JobManagerEventItem{
				conn: c.conn,
			}
			// Task 3 (WO-2026-10-09-001): teruskan nama event untuk kolom `event` di store.
			eventName := strings.TrimPrefix(sub_key, project_app_uuid+".")
			go jobManEvItem.RunGoroutine(cmd, messageObject.Task_id, eventName)
		}(message)
	})
	return unsub, err
}

const (
	START     = 1
	TERMINATE = 2
)

type ActionMap struct {
	Action string
}

func (c *JobManagerEvent) ListenEvent(conn_name string) {
	// Task 3: buka store di proses utama child_process (bukan child_execs_process),
	// jalankan cleanup saat startup, dan pertahankan single-writer per instance.
	_ = stateStoreInstance()

	// Then get the data connection by connection key
	var initPubSubChannel = func() {
		conn := support.Helper.BrokerConnection.GetConnection(conn_name)
		c.conn = conn
		project_app_uuid := support.Helper.ConfigYaml.ConfigData.Uuid
		job_datas := support.Helper.ConfigYaml.ConfigData.Project.Job_datas
		jobs := support.Helper.ConfigYaml.ConfigData.Jobs
		unsubcribes := []func(){}
		if support.Helper.ConfigYaml.ConfigData.End_point == "" {
			for _, v := range jobs {
				jobConfig := v
				sub_key := fmt.Sprint(project_app_uuid, ".", v.Event)
				unsub, err := subscribeAndRunJobEvent(conn, sub_key, jobConfig.Cmd, project_app_uuid, c)
				_ = append(unsubcribes, unsub)
				if err != nil {
					log.Println(err)
				}
			}
		} else {
			// Merged logic: for each job, check both top-level and nested job events
			for _, v := range jobs {
				jobConfig := v
				isMatch := false
				for _, x := range job_datas {
					// Check top-level event
					if x.Event == v.Event {
						isMatch = true
						sub_key := fmt.Sprint(project_app_uuid, ".", v.Event)
						unsub, err := subscribeAndRunJobEvent(conn, sub_key, jobConfig.Cmd, project_app_uuid, c)
						_ = append(unsubcribes, unsub)
						if err != nil {
							log.Println(err)
						}
					}
					// Check nested events
					if x.Nested_jobs != nil {
						for _, nested := range *x.Nested_jobs {
							nestedEvent := fmt.Sprint(x.Event, ".", nested.Event)
							if nestedEvent == v.Event {
								isMatch = true
								sub_key := fmt.Sprint(project_app_uuid, ".", nestedEvent)
								unsub, err := subscribeAndRunJobEvent(conn, sub_key, jobConfig.Cmd, project_app_uuid, c)
								_ = append(unsubcribes, unsub)
								if err != nil {
									log.Println(err)
								}
							}
						}
					}
				}
				if !isMatch {
					fmt.Println("Event :", v.Event, " not register yet. Please check on job manager with app project that you register it.")
				}

			}
		}
		// Subscribe to the "restart" event for the current project application UUID.
		// When the event is triggered, it publishes a "job_item_restart" event to the event bus.
		// This is used to signal that the child process should be restarted.
		// If the subscription fails, an error is logged. Otherwise, the unsubscribe function is added to the list.
		unsub, err := conn.BasicSub(fmt.Sprint(project_app_uuid, ".", "restart"), func(message string) {
			fmt.Println("Received restart event for project_app_uuid:", project_app_uuid)
			support.Helper.EventBus.GetBus().Publish("job_item_restart", nil)
		})
		if err != nil {
			log.Println("Error subscribing to restart event:", err)
		} else {
			_ = append(unsubcribes, unsub)
		}

		// Task 4 (WO-2026-10-09-001): listener control topic `<identity_id>.control`.
		unsubControl := c.listenControlTopic(conn)
		_ = append(unsubcribes, unsubControl)
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
	statusMu    sync.Mutex
	terminate   string
	started     bool
	event       string
}

func (c *JobManagerEventItem) markStarted() {
	c.statusMu.Lock()
	c.started = true
	c.statusMu.Unlock()
}

func (c *JobManagerEventItem) setStatus(status string) {
	c.statusMu.Lock()
	c.Last_status = status
	c.statusMu.Unlock()
}

func (c *JobManagerEventItem) setIntentStatus(status string) {
	c.statusMu.Lock()
	c.terminate = status
	c.Last_status = status
	c.statusMu.Unlock()
}

func (c *JobManagerEventItem) finalStatus(exitCode int) (string, string) {
	c.statusMu.Lock()
	defer c.statusMu.Unlock()
	if c.terminate != "" {
		return c.terminate, support.ProcessStatusTerminated
	}
	if exitCode == 0 {
		c.Last_status = GetStatus().STATUS_FINISH
		return c.Last_status, support.ProcessStatusFinished
	}
	c.Last_status = GetStatus().STATUS_ERROR
	return c.Last_status, support.ProcessStatusFailed
}

func (c *JobManagerEventItem) setStatusIfNoIntent(status string) {
	c.statusMu.Lock()
	if c.terminate == "" {
		c.Last_status = status
	}
	c.statusMu.Unlock()
}

func (c *JobManagerEventItem) lastStatus() string {
	c.statusMu.Lock()
	defer c.statusMu.Unlock()
	return c.Last_status
}

// terminateGracePeriod membaca grace SIGTERM -> SIGKILL dari env
// JOB_ITEM_TERMINATE_GRACE_SECONDS (default 5 detik, SPEC D-8).
func terminateGracePeriod() time.Duration {
	grace := 5
	if v := os.Getenv("JOB_ITEM_TERMINATE_GRACE_SECONDS"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 0 {
			grace = parsed
		}
	}
	return time.Duration(grace) * time.Second
}

// killProcessTree menghentikan seluruh process tree milik cmd (SPEC D-1/D-2/D-10).
// POSIX: SIGTERM ke process group -> grace -> SIGKILL ke process group, idempoten.
// Windows: taskkill /PID <pid> /T /F. Mengembalikan error kill terakhir (nil bila sukses/no-op).
func killProcessTree(cmd *exec.Cmd, taskID string) error {
	if cmd == nil || cmd.Process == nil {
		log.Printf("[TRACE-TERM] timestamp=%s stage=killProcessTree task_id=%q result=no_process", time.Now().UTC().Format(time.RFC3339Nano), taskID)
		return nil
	}
	pid := cmd.Process.Pid
	if runtime.GOOS == "windows" {
		taskkill := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F")
		err := taskkill.Run()
		log.Printf("[TRACE-TERM] timestamp=%s stage=killProcessTree task_id=%q pid=%d method=taskkill err=%v", time.Now().UTC().Format(time.RFC3339Nano), taskID, pid, err)
		return err
	}
	pgid := pid
	// cmd dijalankan dengan Setpgid sehingga pgid == pid shell.
	errTerm := syscall.Kill(-pgid, syscall.SIGTERM)
	log.Printf("[TRACE-TERM] timestamp=%s stage=killProcessTree task_id=%q pgid=%d method=sigterm err=%v", time.Now().UTC().Format(time.RFC3339Nano), taskID, pgid, errTerm)
	if errTerm != nil {
		// Proses/grup sudah mati (ESRCH) -> anggap sukses, idempoten.
		return nil
	}
	grace := terminateGracePeriod()
	if grace == 0 {
		errKill := syscall.Kill(-pgid, syscall.SIGKILL)
		log.Printf("[TRACE-TERM] timestamp=%s stage=killProcessTree task_id=%q pgid=%d method=sigkill_immediate err=%v", time.Now().UTC().Format(time.RFC3339Nano), taskID, pgid, errKill)
		return errKill
	}
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		// kill(-pgid, 0): nil = grup masih ada; ESRCH = grup sudah mati.
		if err := syscall.Kill(-pgid, 0); err != nil {
			log.Printf("[TRACE-TERM] timestamp=%s stage=killProcessTree task_id=%q pgid=%d method=exited_during_grace", time.Now().UTC().Format(time.RFC3339Nano), taskID, pgid)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	errKill := syscall.Kill(-pgid, syscall.SIGKILL)
	log.Printf("[TRACE-TERM] timestamp=%s stage=killProcessTree task_id=%q pgid=%d method=sigkill_after_grace grace=%s err=%v", time.Now().UTC().Format(time.RFC3339Nano), taskID, pgid, grace, errKill)
	return errKill
}

func (c *JobManagerEventItem) RunGoroutine(command string, task_id string, eventName string) {
	c.event = eventName
	project_app_uuid := support.Helper.ConfigYaml.ConfigData.Uuid
	unsub, err := c.conn.Sub(task_id+"_worker", project_app_uuid, func(message string) {
		traceTimestamp := time.Now().UTC().Format(time.RFC3339Nano)
		log.Printf("[TRACE-TERM] timestamp=%s stage=worker_callback task_id=%q topic=%q payload=%s", traceTimestamp, task_id, task_id+"_worker", message)
		messageObject := MessageJson{}
		json.Unmarshal([]byte(message), &messageObject)
		if messageObject.Action == GetStatus().STATUS_TIMEOUT {
			c.setIntentStatus(GetStatus().STATUS_TIMEOUT)
			eventTopic := fmt.Sprint(messageObject.Task_id, "_", "timeout")
			log.Printf("[TRACE-TERM] timestamp=%s stage=eventbus_publish task_id=%q topic=%q action=%q", time.Now().UTC().Format(time.RFC3339Nano), messageObject.Task_id, eventTopic, messageObject.Action)
			support.Helper.EventBus.GetBus().Publish(eventTopic)
		} else if messageObject.Action == GetStatus().STATUS_TERMINATE {
			c.setIntentStatus(GetStatus().STATUS_TERMINATE)
			eventTopic := fmt.Sprint(messageObject.Task_id, "_", "terminate")
			log.Printf("[TRACE-TERM] timestamp=%s stage=eventbus_publish task_id=%q topic=%q action=%q", time.Now().UTC().Format(time.RFC3339Nano), messageObject.Task_id, eventTopic, messageObject.Action)
			support.Helper.EventBus.GetBus().Publish(eventTopic)
		}
	})
	if err != nil {
		log.Println("RunGoroutine :: err :: 23940239409 :: ", err)
	}

	unsubListen, err := c.conn.Sub(fmt.Sprint(task_id, "_", "listen"), project_app_uuid, func(message string) {
		// fmt.Println(sub_key, " :: ", message)
		fmt.Println("message pubsub :: ", message)
		time.Sleep(1 * time.Second)
		c.conn.Pub(fmt.Sprint(task_id, "_", "listen")+".callback", "world")
	})
	if err != nil {
		log.Println("RunGoroutine :: err :: 23940239407 :: ", err)
	}

	// Task 3/4 (WO-2026-10-09-001): catat state RUNNING (store + fallback registry) sebelum proses jalan.
	recordRunning(task_id, c.event)

	c.setStatus(GetStatus().STATUS_FINISH)
	defer func() {
		unsub()
		unsubListen()
		fmt.Println("Closed goroutine")
		time.Sleep(3 * time.Second)
		c.conn.Pub(fmt.Sprint(task_id, "_", "finish"), c.lastStatus())
	}()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", command)
	} else {
		cmd = exec.Command("bash", "-c", command)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	cmd.WaitDelay = 2 * time.Second

	envInvolve := append(os.Environ(),
		// For child processes
		// You need replace :task_id with the actual task ID on the child process
		"JOB_MANAGER_HOST="+support.Helper.ConfigYaml.ConfigData.End_point,
		"JOB_MANAGER_RESULT_URL="+support.Helper.ConfigYaml.ConfigData.End_point+"/api/worker/job_record/result/"+task_id,
		"JOB_MANAGER_UPLOAD_FILE="+support.Helper.ConfigYaml.ConfigData.End_point+"/api/worker/job_record/file/"+task_id,
		"JOB_ITEM_TASK_ID="+task_id,
		"JOB_ITEM_PROJECT_ID="+support.Helper.ConfigYaml.ConfigData.Credential.Project_id,
		"JOB_ITEM_PROJECT_KEY="+support.Helper.ConfigYaml.ConfigData.Credential.Secret_key,
		"JOB_ITEM_MSG_NOTIF_HOST="+os.Getenv("JOB_ITEM_BASE_URL")+"/msg/notif/"+task_id,
	)
	cmd.Env = envInvolve
	c.WatchProcessCMD(cmd, task_id)
}

func (c *JobManagerEventItem) WatchProcessCMD(cmd *exec.Cmd, task_id string) {

	// Create function kill process
	killProcess := func() {
		c.statusMu.Lock()
		started := c.started
		c.statusMu.Unlock()
		if !started {
			log.Printf("[TRACE-TERM] timestamp=%s stage=killProcess task_id=%q result=not_started", time.Now().UTC().Format(time.RFC3339Nano), task_id)
			return
		}
		if cmd.Process == nil {
			log.Printf("[TRACE-TERM] timestamp=%s stage=killProcess_exit task_id=%q pid=0 processpid=0 err=no_process", time.Now().UTC().Format(time.RFC3339Nano), task_id)
			return
		}
		processpid := cmd.Process.Pid
		log.Printf("[TRACE-TERM] timestamp=%s stage=killProcess_entry task_id=%q pid=%d processpid=%d", time.Now().UTC().Format(time.RFC3339Nano), task_id, processpid, processpid)
		err := killProcessTree(cmd, task_id)
		log.Printf("[TRACE-TERM] timestamp=%s stage=killProcess_exit task_id=%q pid=%d processpid=%d err=%v", time.Now().UTC().Format(time.RFC3339Nano), task_id, processpid, processpid, err)
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
		c.setStatusIfNoIntent(GetStatus().STATUS_ERROR)
		recordFinal(task_id, c.event, support.ProcessStatusFailed, -1, err.Error())
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		fmt.Println(err)
		c.setStatusIfNoIntent(GetStatus().STATUS_ERROR)
		recordFinal(task_id, c.event, support.ProcessStatusFailed, -1, err.Error())
		return
	}

	// starting the command
	err = cmd.Start()
	c.markStarted()

	if err != nil {
		// Task 3/4: proses gagal start -> catat final FAILED (exit_code -1) agar tidak tertinggal RUNNING.
		c.setStatusIfNoIntent(GetStatus().STATUS_ERROR)
		recordFinal(task_id, c.event, support.ProcessStatusFailed, -1, err.Error())
		return
	}

	// Task 3: penampung pesan stderr yang sama dengan ERROR_MESSAGE_STDERR.
	var lastErrMu sync.Mutex
	lastErrStr := ""
	isMatchErr := false
	errString := ""

	helper.ShareDataOption(task_id, helper.ShareDataOptions{
		DefaultTTLSeconds: 36000000,
	})

	// no idle-timeout monitor (progress is expected via stdout)

	// stderr reader: collect stderr but do not publish failed until process exit
	var outputReaders sync.WaitGroup
	outputReaders.Add(2)
	go func(conn support.BrokerConnectionInterface) {
		defer outputReaders.Done()
		stderrBuffer := make([]byte, 1024)
		for {
			n2, err2 := stderr.Read(stderrBuffer)

			if err2 != nil {
				fmt.Println("stderr err :: ", err2)
				break
			}

			fmt.Println("UPLOAD_ERROR:", string(stderrBuffer[:n2]))

			// Task 3: simpan chunk error pertama (nilai yang sama dengan ERROR_MESSAGE_STDERR).
			if !isMatchErr {
				isMatchErr = true
				errString = string(stderrBuffer[:n2])
				lastErrMu.Lock()
				lastErrStr = errString
				lastErrMu.Unlock()
				if support.Helper.ConfigYaml.ConfigData.End_point != "" {
					go helper.ShareDataAdd(task_id, "ERROR_MESSAGE_STDERR", errString, 0)
				}
			}

			// Aktivitas stdout/stderr memperbarui last_log_at, tanpa mengubah status final.
			if st := stateStoreInstance(); st != nil {
				_ = st.UpdateActivity(task_id)
			}

			c.setStatusIfNoIntent(GetStatus().STATUS_ERROR)
		}
	}(c.conn)

	// stdout reader: publish progress and reset idle timer
	go func(conn support.BrokerConnectionInterface) {
		defer outputReaders.Done()
		stdoutBuffer := make([]byte, 1024)
		for {
			n, err := stdout.Read(stdoutBuffer)
			if err != nil {
				fmt.Println("stdout err :: ", err)
				break
			}
			fmt.Println("UPLOAD_PROGRESS:", string(stdoutBuffer[:n]))
			// activity observed (progress is handled via stdout/stderr)
			// Task 3: tandai aktivitas stdout pada store.
			if st := stateStoreInstance(); st != nil {
				_ = st.UpdateActivity(task_id)
			}
			if support.Helper.ConfigYaml.ConfigData.End_point != "" {
				conn.Pub(task_id+"_process", string(stdoutBuffer[:n]))
			}
		}
	}(c.conn)

	// Tunggu EOF kedua pipe dahulu (proses menutup stdout/stderr saat exit), baru panggil Wait.
	// Dokumentasi exec.Cmd.StdoutPipe melarang Wait sebelum seluruh pembacaan selesai.
	// Ini memastikan log/error terakhir masuk store sebelum status final ditulis.
	outputReaders.Wait()
	waitErr := cmd.Wait()

	// determine exit code
	exitCode := 0
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				exitCode = status.ExitStatus()
			} else {
				exitCode = 1
			}
		} else {
			exitCode = 1
		}
	} else {
		exitCode = 0
	}

	// publish final status only via RunGoroutine's defer: set Last_status here.
	// Niat terminate/timeout yang sudah tercatat tidak boleh ditimpa exit code (SPEC D-4).
	finalBrokerStatus, finalStoreStatus := c.finalStatus(exitCode)

	// Task 3/4: catat status final (store + fallback registry) SEBELUM `_finish` terpublish.
	lastErrMu.Lock()
	le := lastErrStr
	lastErrMu.Unlock()
	if finalStoreStatus == support.ProcessStatusTerminated {
		recordFinalCompatible(task_id, c.event, finalStoreStatus, exitCode, le)
	} else {
		recordFinal(task_id, c.event, finalStoreStatus, exitCode, le)
	}
	_ = finalBrokerStatus
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

// ---------------------------------------------------------------------------
// Task 3 (WO-2026-10-09-001): persistensi state proses ke SQLite lokal.
// Hanya proses ini (yang menjalankan job) yang menulis store -> single-writer.
// Store dibuka lazy: kegagalan init bersifat non-fatal (persistensi dinonaktifkan,
// alur `_process`/`_finish` tetap berjalan).
// ---------------------------------------------------------------------------

var (
	stateStoreOnce sync.Once
	stateStoreInst *support.ProcessStateStore
)

// stateStoreInstance membuka store sekali (lazy) dan memulai maintenance retensi.
func stateStoreInstance() *support.ProcessStateStore {
	stateStoreOnce.Do(func() {
		st, err := support.InitProcessStateStore("")
		if err != nil {
			log.Println("state store :: init failed, persistence disabled :: ", err)
			return
		}
		support.RegisterProcessStateStore(st)
		stateStoreInst = st
		startStateMaintenance(st)
	})
	return stateStoreInst
}

// stateRetention membaca retensi dari env JOB_ITEM_STATE_RETENTION_HOURS (default 24 jam).
func stateRetention() time.Duration {
	hours := 24
	if v := os.Getenv("JOB_ITEM_STATE_RETENTION_HOURS"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			hours = parsed
		}
	}
	return time.Duration(hours) * time.Hour
}

// startStateMaintenance menjalankan Cleanup retensi saat startup lalu periodik tiap jam.
func startStateMaintenance(st *support.ProcessStateStore) {
	retention := stateRetention()
	runCleanup := func() {
		if n, err := st.Cleanup(retention); err != nil {
			log.Println("state store :: cleanup error :: ", err)
		} else if n > 0 {
			fmt.Println("state store :: cleanup removed", n, "record(s)")
		}
	}
	runCleanup()
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			runCleanup()
		}
	}()
}
