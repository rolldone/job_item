package main

import (
	"fmt"
	"job_item/src/event"
	"job_item/src/helper"
	"job_item/support"
	"log"
	"os"
	"runtime"

	"github.com/fsnotify/fsnotify"
	"github.com/urfave/cli/v2"
)

const VERSION_NUMBER = 3

// For not is not use yet
const VERSION_APP = "v0.1.1"

func main() {
	argsWithoutProg := os.Args[1:]
	if len(argsWithoutProg) > 0 {
		bypass := initCli()
		if !bypass {
			return
		}
	}
	supportSupport := support.SupportConstruct()

	// brokerConnectionSupport := support.BrokerConnectionSupportContruct()
	configYamlSupport := support.ConfigYamlSupportContruct()
	supportSupport.Register(configYamlSupport)

	is_develop, err := helper.IsDevelopment()
	if err != nil {
		fmt.Println(err)
		panic(1)
	}

	if configYamlSupport.ConfigData.Job_item_version_number > VERSION_NUMBER {
		// fmt.Println(configYamlSupport.ConfigData.Job_item_version_number, "::", VERSION_NUMBER)
		fmt.Println("Download New Version :: ", configYamlSupport.ConfigData.Job_item_link)
		err := configYamlSupport.DownloadNewApp()
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
				if event.Has(fsnotify.Write) {
					log.Println("modified file:", event.Name)
					cmd.Process.Kill()
					cmd = nil
					run_child <- "restart"
					fmt.Println("Restart child process")
					is_done_watch = true
				}
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

func initCli() bool {
	var flag string

	app := &cli.App{
		Name:                 "job_item",
		EnableBashCompletion: false,
		Suggest:              false,
		HideHelp:             true,
		HideHelpCommand:      true,
		Commands: []*cli.Command{
			{
				Name: "child_process",
				// Aliases: []string{"c"},
				Usage: "options for config",
				Action: func(ctx *cli.Context) error {
					supportSupport := support.SupportConstruct()

					brokerConnectionSupport := support.BrokerConnectionSupportContruct()
					configYamlSupport := support.ConfigYamlSupportContruct()
					supportSupport.Register(configYamlSupport)

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
						natSupport := support.NatsSupportConstruct(natsBrokerCon)
						brokerConnectionSupport.RegisterConnection(currentConnection["key"].(string), natSupport)
					case "rabbitmq":
						amqpBrokerCon := configYamlSupport.GetRabbitMQBrokenCon(configYamlSupport.GetTypeBrokerCon(currentConnection))
						amqpSupport := support.AMQPSupportConstruct(amqpBrokerCon)
						brokerConnectionSupport.RegisterConnection(currentConnection["key"].(string), amqpSupport)
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
	return flag == ""
}
