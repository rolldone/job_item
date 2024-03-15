package main

import (
	"fmt"
	"job_item/src/event"
	"job_item/support"
	"log"
	"os"
	"runtime"

	"github.com/urfave/cli/v2"
)

func main() {
	// bypass := initCli()
	// if !bypass {
	// 	return
	// }
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

	runtime.Goexit()

	fmt.Println("Exit")
}

const (
	FLAG_JOB    = "job"
	FLAG_CIRCLE = "circle"
)

func initCli() bool {
	var flag string

	app := &cli.App{
		EnableBashCompletion: true,
		Commands: []*cli.Command{
			{
				Name:    "circle",
				Aliases: []string{"c"},
				Usage:   "options for config",
				Subcommands: []*cli.Command{
					{
						Name:  "update",
						Usage: "Update the curernt config",
						Action: func(cCtx *cli.Context) error {
							fmt.Println("Update config : ", cCtx.Args().First())
							return nil
						},
					},
					{
						Name:  "reset",
						Usage: "Reset the current config to be new config",
						Action: func(cCtx *cli.Context) error {
							fmt.Println("Reset config : ", cCtx.Args().First())
							return nil
						},
					},
				},
			},
			{
				Name:    "job",
				Aliases: []string{"t"},
				Usage:   "options for task templates",
				Subcommands: []*cli.Command{
					{
						Name:  "off",
						Usage: "set off the job",
						Action: func(cCtx *cli.Context) error {
							fmt.Println("Set off job : ", cCtx.Args().First())
							return nil
						},
					},
					{
						Name:  "on",
						Usage: "set on the job",
						Action: func(cCtx *cli.Context) error {
							fmt.Println("Set on job : ", cCtx.Args().First())
							return nil
						},
					},
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
	return flag == ""
}
