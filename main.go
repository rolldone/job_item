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
	fmt.Println("configYamlSupport :: ", configYamlSupport.ConfigData)

	supportSupport.Register(configYamlSupport)

	eventBusSupport := support.EventBusConstruct()
	supportSupport.Register(eventBusSupport)

	for _, v := range configYamlSupport.ConfigData.Broker_connections {
		switch v["type"].(string) {
		case "nats":
			// Load the nats library.
			// Init nats broker.
			natsBrokerCon := configYamlSupport.GetNatsBrokerCon(configYamlSupport.GetTypeBrokerCon(v))
			natSupport := support.NatsSupportConstruct(natsBrokerCon)
			brokerConnectionSupport.RegisterConnection(v["key"].(string), natSupport)
		case "rabbitmq":
		}
	}

	supportSupport.Register(brokerConnectionSupport)

	jobManagerEvent := event.JobManagerEventConstruct()
	for _, v := range configYamlSupport.ConfigData.Broker_connections {
		switch v["type"].(string) {
		case "nats":
			// Load the nats library.
			// Init nats broker.
			jobManagerEvent.ListenEvent(v["key"].(string))
		case "rabbitmq":
		}
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
