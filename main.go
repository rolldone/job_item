package main

import (
	"fmt"
	"job_item/src/event"
	"job_item/src/helper"
	"job_item/support"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/urfave/cli/v2"
)

const VERSION_NUMBER = 3

// For not is not use yet
const VERSION_APP = "v0.1.1"

func main() {

	// Check the init cli first is with nested command or not
	// argsWithoutProg := os.Args[1:]
	bypass := initCli()
	if !bypass {
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
		fmt.Println("Download New Version :: ", configYamlSupport.ConfigData.Job_item_link)
		err := configYamlSupport.DownloadNewApp(configYamlSupport.ConfigData.Job_item_version_number)
		if err != nil {
			fmt.Println(err)
			panic(1)
		}
	} else {
		if is_develop {
			fmt.Println("Is Develope")
		}
	}

	run_child := make(chan string, 1)

	is_run := true

	run_child <- "start"
	cmd, err := configYamlSupport.RunChildProcess()
	if err != nil {
		fmt.Println(err)
		panic(1)
	}

	restartProcess := func() {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			log.Fatal(err)
		}
		defer watcher.Close()

		// Add a path.
		err = watcher.Add("config.yaml")
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
				log.Println("event:", event)
				log.Println("modified file:", event.Name)
				err := cmd.Process.Kill()
				if err != nil {
					fmt.Println(err)
					panic(1)
				}
				cmd = nil
				run_child <- "restart"
				fmt.Println("Restart child process")
				is_done_watch = true
				fmt.Println("event.Has(fsnotify.Write) :: ", event.Has(fsnotify.Write))
				// if event.Has(fsnotify.Write) {
				// }
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
		fmt.Println("err :: ", err)
		panic(1)
	}
	fmt.Println("----------------------------------------------------")
	fmt.Println("Hardware Identification")
	fmt.Println("----------------------------------------------------")
	fmt.Println("hostID :: ", hostInfo.HostID)
	fmt.Println("hostname :: ", hostInfo.Hostname)
	fmt.Println("os :: ", hostInfo.OS)
	fmt.Println("platform :: ", hostInfo.Platform)
	fmt.Println("kernelArch :: ", hostInfo.KernelArch)
	fmt.Println("kernelVersion :: ", hostInfo.KernelVersion)
	fmt.Println("----------------------------------------------------")

	for is_run {
		select {
		case gg := <-run_child:
			switch gg {
			case "start":
				err = cmd.Wait()
				if err != nil {
					fmt.Println("The command get : ", err)
				}
			case "restart":
				cmd, err = configYamlSupport.RunChildProcess()
				if err != nil {
					fmt.Println(err)
					panic(1)
				}
				run_child <- "start"
				go restartProcess()
			}
		default:
			fmt.Println("Nothing to do")
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
		fmt.Println("Try restart connection in 5 seconds")
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
			supportSupport := support.SupportConstruct()

			// Retry post data to get authentication from server
			var configYamlSupport *support.ConfigYamlSupport
			retryRequest := true
			for retryRequest {
				confItem, err := support.ConfigYamlSupportContruct(ctx.String("config"))
				configYamlSupport = confItem
				if err != nil {
					retryRequest = true
					time.Sleep(time.Duration(time.Second) * 5)
					fmt.Println("Retry connection")
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
					supportSupport := support.SupportConstruct()

					brokerConnectionSupport := support.BrokerConnectionSupportContruct()

					// Retry post data to get authentication from server
					var configYamlSupport *support.ConfigYamlSupport
					retryRequest := true
					for retryRequest {
						_configYamlSupport, err := support.ConfigYamlSupportContruct(ctx.String("config"))
						configYamlSupport = _configYamlSupport
						if err != nil {
							retryRequest = true
							time.Sleep(time.Duration(time.Second) * 5)
							fmt.Println("Retry connection")
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

					fmt.Println("configYamlSupport", configYamlSupport.ConfigData.Broker_connection)

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
							brokerConnectionSupport.RegisterConnection(currentConnection["key"].(string), natSupport)
							return false
						})
					case "rabbitmq":
						amqpBrokerCon := configYamlSupport.GetRabbitMQBrokenCon(configYamlSupport.GetTypeBrokerCon(currentConnection))
						tryRestartProcess(5, func() bool {
							amqpSupport, err := support.AMQPSupportConstruct(amqpBrokerCon)
							if err != nil {
								return true
							}
							brokerConnectionSupport.RegisterConnection(currentConnection["key"].(string), amqpSupport)
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
						// Load the nats library.
						// Init nats broker.
						jobManagerEvent.ListenEvent(brokCon["key"].(string))
						postOwnInfoEvent.ListenInfoHardware(brokCon["key"].(string))
						postOwnInfoEvent.ListenInfoNetwork(brokCon["key"].(string))
						postOwnInfoEvent.ListenInfoUsage(brokCon["key"].(string))
					case "rabbitmq":
						jobManagerEvent.ListenEvent(brokCon["key"].(string))
						postOwnInfoEvent.ListenInfoHardware(brokCon["key"].(string))
						postOwnInfoEvent.ListenInfoNetwork(brokCon["key"].(string))
						postOwnInfoEvent.ListenInfoUsage(brokCon["key"].(string))
					}

					fmt.Println("Job Item is running :)")

					runtime.Goexit()

					fmt.Println("Exit")
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
