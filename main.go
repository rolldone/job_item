package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	jobitem "job_item/src/controller/JobItem"
	jobmanager "job_item/src/controller/JobManager"
	"job_item/src/event"
	"job_item/src/helper"
	"job_item/support"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gin-gonic/gin"
	"github.com/urfave/cli/v2"
)

const VERSION_NUMBER = 3

// For not is not use yet
const VERSION_APP = "v0.1.1"

func main() {
	// var wg sync.WaitGroup
	// Check the init cli first is with nested command or not
	// argsWithoutProg := os.Args[1:]
	bypass := initCli()
	if !bypass {
		// For child process, we need stop here
		// because the child process will run the command in exec cli.
		// So we need to stop here.
		return
	}

	is_develop, err := helper.IsDevelopment()
	if err != nil {
		fmt.Println(err)
		panic(1)
	}
	var configYamlSupport *support.ConfigYamlSupport = support.Helper.ConfigYaml
	if configYamlSupport.ConfigData.Job_item_version_number > VERSION_NUMBER {
		// fmt.Println(configYamlSupport.ConfigData.Job_item_version_number, "::", VERSION_NUMBER)
		support.Helper.PrintGroupName("Download New Version :: " + configYamlSupport.ConfigData.Job_item_link)
		err := configYamlSupport.DownloadNewApp(configYamlSupport.ConfigData.Job_item_version_number)
		if err != nil {
			support.Helper.PrintErrName(err.Error(), "ERR-25230903100")
			panic(1)
		}
	} else {
		if is_develop {
			support.Helper.PrintGroupName("You are in development mode, so you can change the code and run it again.")
		}
	}

	run_child := make(chan string, 1)

	is_run := true

	run_child <- "start"
	cmdExec, err := configYamlSupport.RunChildExecsProcess()
	if err != nil {
		support.Helper.PrintErrName("Error starting exec process: "+err.Error(), "ERR-303509T3200")
		panic(1)
	}
	cmd, err := configYamlSupport.RunChildProcess()
	if err != nil {
		support.Helper.PrintErrName("Error starting child process: "+err.Error(), "ERR-60350903200")
		panic(1)
	}

	// Watch the config file and restart the child process
	restartProcess := func() {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			support.Helper.PrintErrName("Error creating file watcher: "+err.Error(), "ERR-10351903200")
		}
		defer watcher.Close()

		// Add a path.
		err = watcher.Add(support.Helper.ConfigYaml.Config_path)
		if err != nil {
			log.Fatal(err)
		}
		is_done_watch := false
		// Start listening for events.
		for !is_done_watch {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				support.Helper.PrintGroupName("event: " + event.String())
				support.Helper.PrintGroupName("modified file: " + event.Name)
				configYamlSupport.CloseAllGroupProcesses([]*exec.Cmd{cmd, cmdExec})

				// For cmd is not have child process, so we only wait cmdExec for it
				err = cmdExec.Wait()
				if err != nil {
					support.Helper.PrintErrName("Waiting for exec command : "+err.Error(), "ERR-20350903210")
				}
				time.Sleep(3 * time.Second) // Wait for 3 seconds before restarting
				cmd = nil
				cmdExec = nil
				support.Helper.PrintGroupName("Restart child process...")
				time.Sleep(3 * time.Second) // Wait for 3 seconds before restarting
				run_child <- "restart"
				is_done_watch = true
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Println("error:", err)
			}
		}
	}

	go restartProcess()

	hostInfo, err := support.Helper.HardwareInfo.GetInfoHardware()
	if err != nil {
		support.Helper.PrintErrName("Error getting hardware info: "+err.Error(), "ERR-30350903208")
		panic(1)
	}
	support.Helper.PrintGroupName("--------------------------------------------------------------------------")
	support.Helper.PrintGroupName("Job Item Identity ID :: " + configYamlSupport.ConfigData.Identity_id)
	support.Helper.PrintGroupName("--------------------------------------------------------------------------")
	support.Helper.PrintGroupName("Hardware Identification")
	support.Helper.PrintGroupName("hostID :: " + hostInfo.HostID)
	support.Helper.PrintGroupName("hostname :: " + hostInfo.Hostname)
	support.Helper.PrintGroupName("os :: " + hostInfo.OS)
	support.Helper.PrintGroupName("platform :: " + hostInfo.Platform)
	support.Helper.PrintGroupName("kernelArch :: " + hostInfo.KernelArch)
	support.Helper.PrintGroupName("kernelVersion :: " + hostInfo.KernelVersion)
	support.Helper.PrintGroupName("--------------------------------------------------------------------------")

	// Listen interupt signal
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		for sig := range signalChan {
			support.Helper.PrintGroupName("Received signal: " + sig.String())
			configYamlSupport.CloseAllGroupProcesses([]*exec.Cmd{cmd, cmdExec})
			// For cmd is not have child process, so we only wait cmdExec for it
			err = cmdExec.Wait()
			if err != nil {
				support.Helper.PrintErrName("Waiting for exec command : "+err.Error(), "ERR-20350903511")
			}
			os.Exit(0)
		}
	}()

	for is_run {
		gg := <-run_child
		switch gg {
		case "start":
			support.Helper.PrintGroupName("Starting job_item process")
		case "restart":
			err := initMain(configYamlSupport, configYamlSupport.Config_path)
			if err != nil {
				support.Helper.PrintErrName("Error initializing config yaml support: "+err.Error(), "ERR-30350903201")
				os.Exit(1)
			}

			cmd, err = configYamlSupport.RunChildProcess()
			if err != nil {
				support.Helper.PrintErrName("Error starting child process: "+err.Error(), "ERR-20350903201")
				panic(1)
			}
			cmdExec, err = configYamlSupport.RunChildExecsProcess()
			if err != nil {
				fmt.Println(err)
				panic(1)
			}
			go restartProcess()
			run_child <- "start"
		default:
			support.Helper.PrintGroupName("Nothing to do")
			is_run = false
		}
	}
}

const (
	// defaultBrokerConnectMaxAttempts bounds broker connection retries so the
	// worker can never hang forever when the broker is unreachable.
	defaultBrokerConnectMaxAttempts = 20
	// defaultBrokerConnectBackoff is the delay between broker connection attempts.
	defaultBrokerConnectBackoff = 5 * time.Second
)

// brokerConnectMaxAttempts reads JOB_ITEM_BROKER_CONNECT_MAX_ATTEMPTS.
// Invalid or non-positive values fall back to defaultBrokerConnectMaxAttempts.
func brokerConnectMaxAttempts() int {
	raw := os.Getenv("JOB_ITEM_BROKER_CONNECT_MAX_ATTEMPTS")
	if raw == "" {
		return defaultBrokerConnectMaxAttempts
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		support.Helper.PrintErrName(
			fmt.Sprintf("Invalid JOB_ITEM_BROKER_CONNECT_MAX_ATTEMPTS=%q, falling back to %d", raw, defaultBrokerConnectMaxAttempts),
			"ERR-BROKER-ENV-ATTEMPTS")
		return defaultBrokerConnectMaxAttempts
	}
	return value
}

// brokerConnectBackoff reads JOB_ITEM_BROKER_CONNECT_BACKOFF_SECONDS.
// Invalid or non-positive values fall back to defaultBrokerConnectBackoff.
func brokerConnectBackoff() time.Duration {
	raw := os.Getenv("JOB_ITEM_BROKER_CONNECT_BACKOFF_SECONDS")
	if raw == "" {
		return defaultBrokerConnectBackoff
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		support.Helper.PrintErrName(
			fmt.Sprintf("Invalid JOB_ITEM_BROKER_CONNECT_BACKOFF_SECONDS=%q, falling back to %s", raw, defaultBrokerConnectBackoff),
			"ERR-BROKER-ENV-BACKOFF")
		return defaultBrokerConnectBackoff
	}
	return time.Duration(value) * time.Second
}

func brokerIdentityID(configYamlSupport *support.ConfigYamlSupport) string {
	if identityID := os.Getenv("JOB_ITEM_IDENTITY_ID"); identityID != "" {
		return identityID
	}
	return configYamlSupport.ConfigData.Identity_id
}

// tryRestartProcess retries callback until it succeeds or maxAttempts is reached.
// A nil error means the connection is established. When attempts are exhausted,
// the returned error explains what failed so the caller can exit with a clear log.
func tryRestartProcess(brokerKey string, brokerType string, identityID string, maxAttempts int, backoff time.Duration, callback func() error) error {
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		lastErr = callback()
		if lastErr == nil {
			support.Helper.PrintGroupName(fmt.Sprintf(
				"Broker connected on attempt %d/%d (broker_key=%q broker_type=%q identity_id=%q)",
				attempt, maxAttempts, brokerKey, brokerType, identityID))
			return nil
		}
		if attempt == maxAttempts {
			break
		}
		support.Helper.PrintErrName(fmt.Sprintf(
			"Broker connect attempt %d/%d failed (broker_key=%q broker_type=%q identity_id=%q): %v; retrying in %s",
			attempt, maxAttempts, brokerKey, brokerType, identityID, lastErr, backoff), "ERR-BROKER-RETRY")
		time.Sleep(backoff)
	}
	return fmt.Errorf("giving up broker connection after %d attempts (broker_key=%q broker_type=%q identity_id=%q): %w", maxAttempts, brokerKey, brokerType, identityID, lastErr)
}

func initializeConfigYamlSupport(configYamlSupport *support.ConfigYamlSupport, config string) error {
	totalCountRequest := 3
	var lastErr error
	for attempt := 1; attempt <= totalCountRequest; attempt++ {
		confItem, err := support.ConfigYamlSupportContruct(support.ConfigYamlSupportConstructPropsType{
			Config_path: config,
		})
		if err != nil {
			lastErr = err
			support.Helper.PrintErrName(
				fmt.Sprintf("Error initializing config yaml support (attempt %d/%d): %v", attempt, totalCountRequest, err),
				"ERR-3030903200")
			if attempt < totalCountRequest {
				time.Sleep(time.Duration(time.Second) * 3)
				support.Helper.PrintGroupName("Retry connection")
			}
			continue
		}
		*configYamlSupport = *confItem
		return nil
	}
	return lastErr
}

func initMain(configYamlSupport *support.ConfigYamlSupport, config string) error {
	supportSupport := support.SupportConstruct("Main")

	// Retry post data to get authentication from server
	err := initializeConfigYamlSupport(configYamlSupport, config)
	if err != nil {
		support.Helper.PrintErrName("Error initializing config yaml support: "+err.Error(), "ERR-303509ff03200")
		return err
	}
	supportSupport.Register(configYamlSupport)

	// Initialize event bus support
	eventBusSupport := support.EventBusConstruct()
	supportSupport.Register(eventBusSupport)

	// Initialize broker connection support
	// This will init the broker connection support
	// and register the connection to the broker connection support
	brokerConnectionSupport, err := initBrokerConnections(configYamlSupport)
	if err != nil {
		return err
	}
	supportSupport.Register(brokerConnectionSupport)

	// Listen signal shutdown from child and child exec process
	brokerKey, err := configYamlSupport.GetBrokerConnectionKey()
	if err != nil {
		return err
	}
	conn := brokerConnectionSupport.GetConnection(brokerKey)
	if conn == nil {
		return fmt.Errorf("no broker connection registered for key %q", brokerKey)
	}
	configYamlSupport.ListenForShutdownFromConn(conn)

	// Register gin support
	ginSupport := support.GinConstruct()
	supportSupport.Register(ginSupport)
	ginInitialize(ginSupport.Router)
	return nil
}

func initCli() bool {
	var flag string

	flagConfig := []cli.Flag{
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			Value:   "config.yaml",
			Usage:   "configuration file",
			EnvVars: []string{"CONFIG_PATH"},
		},
	}

	app := &cli.App{
		Name:                 "job_item",
		EnableBashCompletion: false,
		Suggest:              false,
		HideHelp:             true,
		HideHelpCommand:      true,
		Flags:                flagConfig,

		// This is without nested command
		// Example job_item --config=/var/www/html/config.yaml
		Action: func(ctx *cli.Context) error {
			var configYamlSupport support.ConfigYamlSupport
			if err := initMain(&configYamlSupport, ctx.String("config")); err != nil {
				support.Helper.PrintErrName("Worker startup failed: "+err.Error(), "ERR-30350903209")
				return err
			}
			return nil
		},

		// This is with nested command
		// Example job_item child_process --config=/var/www/html/config.yaml
		Commands: []*cli.Command{
			{
				Flags: flagConfig,
				Name:  "upload",
				Usage: "for upload the file to job manager server",
				Action: func(ctx *cli.Context) error {
					// Check if JOB_MANAGER_UPLOAD_FILE environment variable is set
					uploadURL := os.Getenv("JOB_MANAGER_UPLOAD_FILE")
					if uploadURL == "" {
						fmt.Printf("Error: JOB_MANAGER_UPLOAD_FILE environment variable is not set\n")
						fmt.Printf("Please set JOB_MANAGER_UPLOAD_FILE (e.g., export JOB_MANAGER_UPLOAD_FILE=http://job-manager:5280/api/worker/job_record/file/task-id)\n")
						return fmt.Errorf("JOB_MANAGER_UPLOAD_FILE environment variable is required")
					}

					// Check if JOB_ITEM_PROJECT_KEY environment variable is set for authentication
					projectKey := os.Getenv("JOB_ITEM_PROJECT_KEY")
					if projectKey == "" {
						fmt.Printf("Error: JOB_ITEM_PROJECT_KEY environment variable is not set\n")
						fmt.Printf("Please set JOB_ITEM_PROJECT_KEY (e.g., export JOB_ITEM_PROJECT_KEY=your-secret-key)\n")
						return fmt.Errorf("JOB_ITEM_PROJECT_KEY environment variable is required")
					}

					// Get file path from command line arguments
					args := ctx.Args().Slice()
					if len(args) == 0 {
						fmt.Printf("Error: No file path provided\n")
						fmt.Printf("Usage: ./job_item upload <file_path>\n")
						fmt.Printf("Example: ./job_item upload /path/to/your/file.pdf\n")
						return fmt.Errorf("file path is required")
					}

					filePath := args[0]

					// Check if file exists
					if _, err := os.Stat(filePath); os.IsNotExist(err) {
						fmt.Printf("Error: File does not exist: %s\n", filePath)
						return fmt.Errorf("file not found: %s", filePath)
					}

					// Upload the file
					fmt.Printf("Uploading file: %s\n", filePath)
					fmt.Printf("Upload URL: %s\n", uploadURL)

					err := uploadFile(filePath, uploadURL, projectKey)
					if err != nil {
						fmt.Printf("✗ Upload failed: %v\n", err)
						return err
					}

					fmt.Printf("✓ File uploaded successfully: %s\n", filepath.Base(filePath))

					flag = "upload"
					return nil
				},
			},
			{
				Flags: flagConfig,
				Name:  "save",
				// Aliases: []string{"c"},
				Usage: "for save stdout/stderr and send to notif (use: command 2>&1 | ./job_item . save)",
				Action: func(ctx *cli.Context) error {
					// Read from stdin (piped input)
					scanner := bufio.NewScanner(os.Stdin)
					var output strings.Builder

					// fmt.Println("Reading piped input...")
					for scanner.Scan() {
						line := scanner.Text()
						fmt.Println(line) // Print to stdout as well
						output.WriteString(line + "\n")
					}

					// if err := scanner.Err(); err != nil {
					// 	errorMsg := fmt.Sprintf("Error reading stdin: %v\n", err)
					// 	fmt.Print(errorMsg)
					// 	output.WriteString(errorMsg) // Record the error in output too
					// 	// Don't return here, continue to send the notification with whatever we captured
					// }

					capturedOutput := output.String()
					// fmt.Println("Osget env ERROR_MESSAGE_STDERR", os.Getenv("ERROR_MESSAGE_STDERR"))

					sDatas, _ := helper.ShareDataGet(os.Getenv("JOB_ITEM_TASK_ID"), "ERROR_MESSAGE_STDERR")
					if len(sDatas) > 0 {
						capturedOutput += strings.Join(sDatas, "")
					}

					// Get task ID from environment variable
					taskId := os.Getenv("JOB_ITEM_TASK_ID")
					if taskId == "" {
						fmt.Printf("Error: JOB_ITEM_TASK_ID environment variable is not set\n")
						fmt.Printf("Please set JOB_ITEM_TASK_ID (e.g., export JOB_ITEM_TASK_ID=your-task-id)\n")
						return fmt.Errorf("JOB_ITEM_TASK_ID environment variable is required")
					}

					// Get notification URL from environment variable
					baseNotifURL := os.Getenv("JOB_ITEM_MSG_NOTIF_HOST")
					if baseNotifURL == "" {
						fmt.Printf("Error: JOB_ITEM_MSG_NOTIF_HOST environment variable is not set\n")
						fmt.Printf("Please set JOB_ITEM_MSG_NOTIF_HOST (e.g., export JOB_ITEM_MSG_NOTIF_HOST=http://localhost:8080/msg/notif)\n")
						return fmt.Errorf("JOB_ITEM_MSG_NOTIF_HOST environment variable is required")
					}

					// Build notification URL with task ID
					notifURL := baseNotifURL
					notifPayload := map[string]string{
						"msg": capturedOutput,
					}

					jsonData, err := json.Marshal(notifPayload)
					if err != nil {
						fmt.Printf("Error marshaling notification: %v\n", err)
						return err
					}

					fmt.Printf("Sending notification to: %s\n", notifURL)
					resp, err := http.Post(notifURL, "application/json", bytes.NewBuffer(jsonData))
					if err != nil {
						fmt.Printf("Error sending notification: %v\n", err)
						return err
					}
					defer resp.Body.Close()

					if resp.StatusCode == http.StatusOK {
						fmt.Printf("✓ Notification sent successfully (Task ID: %s)\n", taskId)
					} else {
						fmt.Printf("✗ Notification failed with status: %d\n", resp.StatusCode)
					}

					flag = "save"
					return nil
				},
			},
			{
				Flags: flagConfig,
				Name:  "child_process",
				// Aliases: []string{"c"},
				Usage: "options for config",
				Action: func(ctx *cli.Context) error {
					flag = "child_process"

					supportSupport := support.SupportConstruct("Child")

					// Retry post data to get authentication from server
					var configYamlSupport support.ConfigYamlSupport
					err := initializeConfigYamlSupport(&configYamlSupport, ctx.String("config"))
					if err != nil {
						support.Helper.PrintErrName("Error initializing config yaml support: "+err.Error(), "ERR-30350906201")
						return err
					}
					supportSupport.Register(&configYamlSupport)

					eventBusSupport := support.EventBusConstruct()
					supportSupport.Register(eventBusSupport)

					harwareInfoSuppport := support.HardwareInfoSupportConstruct()
					supportSupport.Register(harwareInfoSuppport)

					// Print the broker connection details in a structured format for debugging and verification
					brokerConnection := configYamlSupport.ConfigData.Broker_connection
					support.Helper.PrintGroupName("Broker Connection Details:")
					for key, value := range brokerConnection {
						support.Helper.PrintGroupName(fmt.Sprintf("  %s: %v", key, value))
					}

					// Initialize broker connection support
					// This will init the broker connection support
					// and register the connection to the broker connection support
					brokerConnectionSupport, err := initBrokerConnections(&configYamlSupport)
					if err != nil {
						support.Helper.PrintErrName("Error initializing broker connection: "+err.Error(), "ERR-30350906202")
						return err
					}
					supportSupport.Register(brokerConnectionSupport)

					// Check the own event have regsiter to job manager event
					jobManagerEvent := event.JobManagerEventConstruct()
					postOwnInfoEvent := event.ListenOwnHardwareInfoEvent{}
					brokCon := configYamlSupport.ConfigData.Broker_connection
					brokerType, err := support.GetConfigString(brokCon, "type")
					if err != nil {
						return err
					}
					brokerKey, err := configYamlSupport.GetBrokerConnectionKey()
					if err != nil {
						return err
					}
					switch brokerType {
					case "nats":
						// Init nats broker.
						jobManagerEvent.ListenEvent(brokerKey)
						if support.Helper.ConfigYaml.ConfigData.End_point != "" {
							postOwnInfoEvent.ListenInfoHardware(brokerKey)
							postOwnInfoEvent.ListenInfoNetwork(brokerKey)
							postOwnInfoEvent.ListenInfoUsage(brokerKey)
						}
					case "rabbitmq":
						// Init rabbitmq broker.
						jobManagerEvent.ListenEvent(brokerKey)
						if support.Helper.ConfigYaml.ConfigData.End_point != "" {
							postOwnInfoEvent.ListenInfoHardware(brokerKey)
							postOwnInfoEvent.ListenInfoNetwork(brokerKey)
							postOwnInfoEvent.ListenInfoUsage(brokerKey)
						}
					case "redis":
						// Init redis broker.
						jobManagerEvent.ListenEvent(brokerKey)
						if support.Helper.ConfigYaml.ConfigData.End_point != "" {
							postOwnInfoEvent.ListenInfoHardware(brokerKey)
							postOwnInfoEvent.ListenInfoNetwork(brokerKey)
							postOwnInfoEvent.ListenInfoUsage(brokerKey)
						}
					}

					supportSupport.PrintGroupName("Job Item is running :)")

					// This function listens for the "job_item_restart" event on the event bus.
					// When the event is triggered, it attempts to save the current configuration file (config.yaml)
					// without making any changes to its content. This is used to restart the process because
					// the parent process is watching the config.yaml file for changes. If saving fails, an error message is logged.
					restartProcessFromEventBus := func() {
						eventBusSupport := support.Helper.EventBus
						err := eventBusSupport.GetBus().SubscribeOnce("job_item_restart", func(data interface{}) {
							support.Helper.PrintGroupName("Restart child process from event bus")
							err := saveWithoutChange(support.Helper.ConfigYaml.Config_path)
							if err != nil {
								support.Helper.PrintErrName("ERROR: "+err.Error(), "ERR-80350903201")
								// Handle the error appropriately
							}
						})
						if err != nil {
							support.Helper.PrintErrName("Error subscribing to job_item_restart event: "+err.Error(), "ERR-40350903201")
							return
						}
					}

					go restartProcessFromEventBus()

					// --- Signal Handling ---
					sigs := make(chan os.Signal, 1)
					signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
					<-sigs
					support.Helper.PrintGroupName("Received signal, shutting down gracefully...")
					return nil
				},
			},
			{
				Flags: flagConfig,
				Name:  "child_execs_process",
				// Aliases: []string{"e"},
				Usage: "run execs process",
				Action: func(ctx *cli.Context) error {
					flag = "child_execs_process"
					supportSupport := support.SupportConstruct("Exec")
					// Initialize without request to server
					var configYamlSupport support.ConfigYamlSupport
					err := initializeConfigYamlSupport(&configYamlSupport, ctx.String("config"))
					if err != nil {
						support.Helper.PrintErrName("Error initializing config yaml support: "+err.Error(), "ERR-3035090233202")
						return err
					}

					supportSupport.Register(&configYamlSupport)

					// Initialize event bus support
					eventBusSupport := support.EventBusConstruct()
					supportSupport.Register(eventBusSupport)

					// Initialize broker connection support
					// This will init the broker connection support
					// and register the connection to the broker connection support
					brokerConnectionSupport, err := initBrokerConnections(&configYamlSupport)
					if err != nil {
						support.Helper.PrintErrName("Error initializing broker connection: "+err.Error(), "ERR-3035090233203")
						return err
					}
					supportSupport.Register(brokerConnectionSupport)

					var cmdExecArr []*exec.Cmd
					configYamlSupport.RunExecsProcess(&cmdExecArr)
					if len(cmdExecArr) == 0 {
						fmt.Println("Nothing to do")
						return nil
					}

					// Start Listening for signals to gracefully shut down the process
					sig := make(chan os.Signal, 1)
					signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
					<-sig
					support.Helper.PrintGroupName("Received signal, shutting down gracefully... ")
					for _, cmdExec := range cmdExecArr {
						support.Helper.PrintGroupName("PID Exec: " + strconv.Itoa(cmdExec.Process.Pid))
						// Kirim SIGTERM
						configYamlSupport.CloseAllGroupProcesses([]*exec.Cmd{cmdExec})
					}
					return nil
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
	// It mean bypass
	return flag == ""
}

func saveWithoutChange(filePath string) error {
	// Read the current content of the file
	data, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Printf("ERROR: Failed to read %s: %v\n", filePath, err)
		return err
	}

	// Write the same content back to the file
	err = os.WriteFile(filePath, data, 0644)
	if err != nil {
		fmt.Printf("ERROR: Failed to save %s: %v\n", filePath, err)
		return err
	}

	fmt.Printf("Successfully saved %s without changes\n", filePath)
	return nil
}

func uploadFile(filePath, uploadURL, projectKey string) error {
	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("error opening file: %v", err)
	}
	defer file.Close()

	// Stream upload using io.Pipe to avoid buffering entire request in memory
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	// Create the HTTP request with the pipe reader as body
	req, err := http.NewRequest("POST", uploadURL, pr)
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+projectKey)

	// Configure client timeout (env JOB_ITEM_UPLOAD_TIMEOUT_SECONDS, default 300 seconds)
	// Default 300s (5 minutes) to handle large uploads; set to 0 to disable timeout.
	uploadTimeoutSec := 300
	if v := os.Getenv("JOB_ITEM_UPLOAD_TIMEOUT_SECONDS"); v != "" {
		if vi, err := strconv.Atoi(v); err == nil {
			uploadTimeoutSec = vi
		}
	}
	client := &http.Client{}
	if uploadTimeoutSec > 0 {
		client.Timeout = time.Duration(uploadTimeoutSec) * time.Second
	}

	// Start goroutine to write multipart data into the pipe
	go func() {
		defer pw.Close()
		defer writer.Close()

		part, err := writer.CreateFormFile("file", filepath.Base(filePath))
		if err != nil {
			pw.CloseWithError(err)
			return
		}

		// reset file read position
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			pw.CloseWithError(err)
			return
		}

		fileInfo, err := file.Stat()
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		totalSize := fileInfo.Size()

		buf := make([]byte, 32*1024)
		var readBytes int64 = 0
		lastPct := -1
		for {
			n, rerr := file.Read(buf)
			if n > 0 {
				wn, werr := part.Write(buf[:n])
				if werr != nil {
					pw.CloseWithError(werr)
					return
				}
				readBytes += int64(wn)
				pct := 0
				if totalSize > 0 {
					pct = int((readBytes * 100) / totalSize)
				}
				if pct != lastPct {
					lastPct = pct
					fmt.Printf("PROGRESS: %d%%\n", pct)
				}
			}
			if rerr != nil {
				if rerr == io.EOF {
					break
				}
				pw.CloseWithError(rerr)
				return
			}
		}
	}()

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error uploading file: %v", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response body: %v", err)
	}

	// Check response status
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse JSON response
	var response struct {
		Status     string `json:"status"`
		StatusCode int    `json:"status_code"`
		Return     struct {
			FileInfo struct {
				ID            int    `json:"id"`
				JobRecordUUID string `json:"job_record_uuid"`
				FileName      string `json:"file_name"`
			} `json:"file_info"`
			DownloadURL    string `json:"download_url"`
			SecurityNotice string `json:"security_notice"`
			ExpiryNotice   string `json:"expiry_notice"`
		} `json:"return"`
	}

	err = json.Unmarshal(body, &response)
	if err != nil {
		return fmt.Errorf("error parsing response JSON: %v", err)
	}

	// Display the important information
	fmt.Printf("\n--- Upload Response ---\n")
	fmt.Printf("Download URL: %s\n", response.Return.DownloadURL)
	fmt.Printf("Security Notice: %s\n", response.Return.SecurityNotice)
	fmt.Printf("Expiry Notice: %s\n", response.Return.ExpiryNotice)
	fmt.Printf("----------------------\n")

	return nil
}

func ginInitialize(router *gin.Engine) {
	msgNotifController := jobmanager.NewMsgNotifController()
	router.POST("/msg/notif/:task_id", msgNotifController.AddNotif)

	// Share data controller (temporary, in-memory)
	shareDataController := jobmanager.NewShareDataController()
	router.POST("/msg/share/data/:task_id", shareDataController.AddShareData)
	router.GET("/msg/share/data/:task_id/:key", shareDataController.GetShareData)

	// This is for local app Communication
	jobGroup := router.Group("/job")
	{
		jobGroup.POST("/create", jobitem.CreateJobHandler)
	}
}

func initBrokerConnections(configYamlSupport *support.ConfigYamlSupport) (*support.BrokerConnectionSupport, error) {
	brokerConnectionSupport := support.BrokerConnectionSupportContruct()
	currentConnection := configYamlSupport.ConfigData.Broker_connection

	// Check if broker connection is properly configured
	if currentConnection == nil {
		return nil, fmt.Errorf("broker connection config is missing; please check the job manager server connection")
	}

	// Check if broker type is specified and is a non-empty string
	brokerType, err := support.GetConfigString(currentConnection, "type")
	if err != nil {
		return nil, fmt.Errorf("broker connection type is invalid: %w; please configure the broker type (nats, rabbitmq, or redis) in the Job Manager", err)
	}

	// Check if broker key is specified and is a non-empty string
	brokerKey, err := configYamlSupport.GetBrokerConnectionKey()
	if err != nil {
		return nil, fmt.Errorf("broker connection key is invalid: %w; please configure the broker connection key in the Job Manager", err)
	}

	maxAttempts := brokerConnectMaxAttempts()
	backoff := brokerConnectBackoff()
	identityID := brokerIdentityID(configYamlSupport)

	brokerCon, err := configYamlSupport.GetTypeBrokerCon(currentConnection)
	if err != nil {
		return nil, fmt.Errorf("invalid broker connection config: %w", err)
	}

	var connectErr error
	switch brokerType {
	case "nats":
		natsBrokerCon := configYamlSupport.GetNatsBrokerCon(brokerCon)
		connectErr = tryRestartProcess(brokerKey, brokerType, identityID, maxAttempts, backoff, func() error {
			natSupport, err := support.NatsSupportConstruct(natsBrokerCon)
			if err != nil {
				return err
			}
			var gg *support.NatsSupport = &natSupport
			brokerConnectionSupport.RegisterConnection(brokerKey, gg)
			return nil
		})
	case "rabbitmq":
		amqpBrokerCon := configYamlSupport.GetRabbitMQBrokenCon(brokerCon)
		connectErr = tryRestartProcess(brokerKey, brokerType, identityID, maxAttempts, backoff, func() error {
			amqpSupport, err := support.AMQPSupportConstruct(amqpBrokerCon)
			if err != nil {
				return err
			}
			var gg *support.AMQPSupport = amqpSupport
			brokerConnectionSupport.RegisterConnection(brokerKey, gg)
			return nil
		})
	case "redis":
		redisBrokerCon := configYamlSupport.GetRedisBrokerCon(brokerCon)
		connectErr = tryRestartProcess(brokerKey, brokerType, identityID, maxAttempts, backoff, func() error {
			redisSupport, err := support.NewRedisSupportConstruct(redisBrokerCon)
			if err != nil {
				return err
			}
			var gg *support.RedisSupport = redisSupport
			brokerConnectionSupport.RegisterConnection(brokerKey, gg)
			return nil
		})
	default:
		return nil, fmt.Errorf("unsupported broker type %q (supported: nats, rabbitmq, redis); please select a valid broker connection in the Job Manager", brokerType)
	}
	if connectErr != nil {
		return nil, connectErr
	}
	return brokerConnectionSupport, nil
}
