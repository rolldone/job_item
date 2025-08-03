package main

import (
	"fmt"
	"job_item/src/event"
	"job_item/src/helper"
	"job_item/support"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
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

	configYamlSupport := support.Helper.ConfigYaml
	if configYamlSupport.ConfigData.Job_item_version_number > VERSION_NUMBER {
		// fmt.Println(configYamlSupport.ConfigData.Job_item_version_number, "::", VERSION_NUMBER)
		support.Helper.PrintGroupName("Download New Version :: " + configYamlSupport.ConfigData.Job_item_link)
		err := configYamlSupport.DownloadNewApp(configYamlSupport.ConfigData.Job_item_version_number)
		if err != nil {
			support.Helper.PrintErrName(err.Error())
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
		support.Helper.PrintErrName("Error starting exec process: " + err.Error())
		panic(1)
	}
	cmd, err := configYamlSupport.RunChildProcess()
	if err != nil {
		support.Helper.PrintErrName("Error starting child process: " + err.Error())
		panic(1)
	}

	// Watch the config file and restart the child process
	restartProcess := func() {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			support.Helper.PrintErrName("Error creating file watcher: " + err.Error())
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
					support.Helper.PrintErrName("Error waiting for exec command: " + err.Error())
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
		support.Helper.PrintErrName("Error getting hardware info: " + err.Error())
		panic(1)
	}
	support.Helper.PrintGroupName("----------------------------------------------------")
	support.Helper.PrintGroupName("Hardware Identification")
	support.Helper.PrintGroupName("hostID :: " + hostInfo.HostID)
	support.Helper.PrintGroupName("hostname :: " + hostInfo.Hostname)
	support.Helper.PrintGroupName("os :: " + hostInfo.OS)
	support.Helper.PrintGroupName("platform :: " + hostInfo.Platform)
	support.Helper.PrintGroupName("kernelArch :: " + hostInfo.KernelArch)
	support.Helper.PrintGroupName("kernelVersion :: " + hostInfo.KernelVersion)
	support.Helper.PrintGroupName("----------------------------------------------------")

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
				support.Helper.PrintErrName("Error waiting for exec command: " + err.Error())
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
			cmd, err = configYamlSupport.RunChildProcess()
			if err != nil {
				support.Helper.PrintErrName("Error starting child process: " + err.Error())
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

func tryRestartProcess(waitingRecursive int, callback func() bool) {
	retryConn := true
	for retryConn {
		retryConn = callback()
		if !retryConn {
			break
		}
		support.Helper.PrintGroupName("Try restart connection in 5 seconds")
		time.Sleep(time.Duration(time.Second) * time.Duration(waitingRecursive))
	}
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
			supportSupport := support.SupportConstruct("Main")

			// Retry post data to get authentication from server
			var configYamlSupport *support.ConfigYamlSupport
			retryRequest := true
			for retryRequest {
				confItem, err := support.ConfigYamlSupportContruct(support.ConfigYamlSupportConstructPropsType{
					RequestToServer: false,
					Config_path:     ctx.String("config"),
				})
				configYamlSupport = confItem
				if err != nil {
					retryRequest = true
					time.Sleep(time.Duration(time.Second) * 5)
					support.Helper.PrintGroupName("Retry connection")
					continue
				}
				retryRequest = false
				supportSupport.Register(configYamlSupport)
			}
			return nil
		},

		// This is with nested command
		// Example job_item child_process --config=/var/www/html/config.yaml
		Commands: []*cli.Command{
			{
				Flags: flagConfig,
				Name:  "child_process",
				// Aliases: []string{"c"},
				Usage: "options for config",
				Action: func(ctx *cli.Context) error {
					flag = "child_process"

					supportSupport := support.SupportConstruct("Child")

					brokerConnectionSupport := support.BrokerConnectionSupportContruct()

					// Retry post data to get authentication from server
					var configYamlSupport *support.ConfigYamlSupport
					retryRequest := true
					for retryRequest {
						_configYamlSupport, err := support.ConfigYamlSupportContruct(support.ConfigYamlSupportConstructPropsType{
							RequestToServer: true,
							Config_path:     ctx.String("config"),
						})
						configYamlSupport = _configYamlSupport
						if err != nil {
							retryRequest = true
							time.Sleep(time.Duration(time.Second) * 5)
							support.Helper.PrintGroupName("Retry connection")
							continue
						}
						retryRequest = false
						supportSupport.Register(configYamlSupport)
						break
					}

					eventBusSupport := support.EventBusConstruct()
					supportSupport.Register(eventBusSupport)

					harwareInfoSuppport := support.HardwareInfoSupportConstruct()
					supportSupport.Register(harwareInfoSuppport)

					support.Helper.PrintGroupName("configYamlSupport: " + fmt.Sprintf("%v", configYamlSupport.ConfigData.Broker_connection))

					currentConnection := configYamlSupport.ConfigData.Broker_connection
					switch currentConnection["type"].(string) {
					case "nats":
						// Load the nats library.
						// Init nats broker.
						natsBrokerCon := configYamlSupport.GetNatsBrokerCon(configYamlSupport.GetTypeBrokerCon(currentConnection))
						tryRestartProcess(5, func() bool {
							natSupport, err := support.NatsSupportConstruct(natsBrokerCon)
							if err != nil {
								return true
							}
							var gg *support.NatsSupport = &natSupport
							brokerConnectionSupport.RegisterConnection(currentConnection["key"].(string), gg)
							return false
						})
					case "rabbitmq":
						amqpBrokerCon := configYamlSupport.GetRabbitMQBrokenCon(configYamlSupport.GetTypeBrokerCon(currentConnection))
						tryRestartProcess(5, func() bool {
							amqpSupport, err := support.AMQPSupportConstruct(amqpBrokerCon)
							if err != nil {
								return true
							}
							var gg *support.AMQPSupport = amqpSupport
							brokerConnectionSupport.RegisterConnection(currentConnection["key"].(string), gg)
							return false
						})
					case "redis":
						redisBrokerCon := configYamlSupport.GetRedisBrokerCon(configYamlSupport.GetTypeBrokerCon(currentConnection))
						tryRestartProcess(5, func() bool {
							redisSupport, err := support.NewRedisSupportConstruct(redisBrokerCon)
							if err != nil {
								return true
							}
							var gg *support.RedisSupport = redisSupport
							brokerConnectionSupport.RegisterConnection(currentConnection["key"].(string), gg)
							return false
						})
					}

					supportSupport.Register(brokerConnectionSupport)

					// Check the own event have regsiter to job manager event
					jobManagerEvent := event.JobManagerEventConstruct()
					postOwnInfoEvent := event.ListenOwnHardwareInfoEvent{}
					brokCon := configYamlSupport.ConfigData.Broker_connection
					switch brokCon["type"].(string) {
					case "nats":
						// Init nats broker.
						jobManagerEvent.ListenEvent(brokCon["key"].(string))
						postOwnInfoEvent.ListenInfoHardware(brokCon["key"].(string))
						postOwnInfoEvent.ListenInfoNetwork(brokCon["key"].(string))
						postOwnInfoEvent.ListenInfoUsage(brokCon["key"].(string))
					case "rabbitmq":
						// Init rabbitmq broker.
						jobManagerEvent.ListenEvent(brokCon["key"].(string))
						postOwnInfoEvent.ListenInfoHardware(brokCon["key"].(string))
						postOwnInfoEvent.ListenInfoNetwork(brokCon["key"].(string))
						postOwnInfoEvent.ListenInfoUsage(brokCon["key"].(string))
					case "redis":
						// Init redis broker.
						jobManagerEvent.ListenEvent(brokCon["key"].(string))
						postOwnInfoEvent.ListenInfoHardware(brokCon["key"].(string))
						postOwnInfoEvent.ListenInfoNetwork(brokCon["key"].(string))
						postOwnInfoEvent.ListenInfoUsage(brokCon["key"].(string))
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
								support.Helper.PrintErrName("ERROR: " + err.Error())
								// Handle the error appropriately
							}
						})
						if err != nil {
							support.Helper.PrintErrName("Error subscribing to job_item_restart event: " + err.Error())
							return
						}
					}

					go restartProcessFromEventBus()

					// Register gin support
					ginSupport := support.GinConstruct()
					supportSupport.Register(ginSupport)

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
					configYamlSupport, err := support.ConfigYamlSupportContruct(support.ConfigYamlSupportConstructPropsType{
						RequestToServer: false,
						Config_path:     ctx.String("config"),
					})
					if err != nil {
						support.Helper.PrintErrName("Error initializing config yaml support: " + err.Error())
						return err
					}

					supportSupport.Register(configYamlSupport)

					cmdExecArr := configYamlSupport.RunExecsProcess()
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

// Example usage in your main function or relevant part of the code:
// data := []byte("your YAML content here") // Replace with actual YAML content
// err := saveConfig("config.yaml", data)
// if err != nil {
//     fmt.Printf("ERROR: %v\n", err)
//     // Handle the error appropriately
// }
