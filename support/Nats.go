package support

import (
	"fmt"
	"log"
	"strconv"
	"time"

	nats "github.com/nats-io/nats.go"
)

type NatsConfInfo struct {
	NATS_HOST string
	NATS_PORT int
}

func NatsSupportConstruct(props NatsBrokerConnection) (*NatsSupport, error) {
	gg := NatsSupport{
		natConfInfo: props,
	}
	err := gg.ConnectPubSub()
	return &gg, err
}

type NatsSupport struct {
	nc          *nats.Conn
	natConfInfo NatsBrokerConnection
	key         string
}

func (c *NatsSupport) ConnectPubSub() error {
	natsHost := c.natConfInfo.Host
	natsPort := c.natConfInfo.Port
	natsAuthType := c.natConfInfo.Auth_type
	natsUser := c.natConfInfo.User
	natsPassword := c.natConfInfo.Password
	natsToken := c.natConfInfo.Token

	// Connect to a server
	url := fmt.Sprint("nats://" + natsHost + ":" + strconv.Itoa(natsPort))
	switch natsAuthType {
	case "token":
		url = fmt.Sprint("nats://" + natsToken + "@" + natsHost + ":" + strconv.Itoa(natsPort))
	case "user_password":
		url = fmt.Sprint("nats://" + natsUser + ":" + natsPassword + "@" + natsHost + ":" + strconv.Itoa(natsPort))
	case "user_password_bcrypt":
		url = fmt.Sprint("nats://" + natsUser + ":" + natsPassword + "@" + natsHost + ":" + strconv.Itoa(natsPort))
	}
	fmt.Println("Nats Connection inf :: ", url)
	nc, err := nats.Connect(url)
	c.nc = nc

	if err != nil {
		fmt.Println("Nats error :: ", err.Error())
	}

	// Wanna tester add publish at below

	return err
}

// Interface from BrokerConnectionInterface
func (c *NatsSupport) Pub(topic string, msg string) {
	c.nc.Publish(topic, []byte(msg))
}

// Interface from BrokerConnectionInterface
func (c *NatsSupport) Sub(topic string, group_id string, callback func(message string)) (func(), error) {
	unsubribce, err := c.nc.QueueSubscribe(topic, group_id, func(msg *nats.Msg) {
		callback(string(msg.Data))
	})
	if err != nil {
		log.Fatal(err)
		return nil, err
	}
	return func() {
		unsubribce.Unsubscribe()
	}, nil
}

// Interface from BrokerConnectionInterface
func (c *NatsSupport) SubSync(uuidItem string, group_id string, callback func(message string, err error), opts SubSyncOpts) error {
	unsubribce, err := c.nc.QueueSubscribeSync(uuidItem, group_id)
	if err != nil {
		log.Fatal(err)
		return err
	}
	messageCount := 0
	maxMessages := opts.Timeout // Maximum number of timeout process
	for messageCount < maxMessages {
		// Wait for a message
		msg, err := unsubribce.NextMsg(1 * time.Second) // Timeout after 5 seconds if no message
		if err != nil {
			if err == nats.ErrTimeout {
				fmt.Println("Timed out waiting for a message.", (messageCount + 1))
				messageCount++
				continue
			}
			log.Fatal(err)
		}

		// Process the received message
		fmt.Printf("\n unSubscribeFinish: %s\n", msg.Data)
		callback(string(msg.Data), nil)
		unsubribce.Unsubscribe()
		break
	}
	if messageCount >= maxMessages {
		unsubribce.Unsubscribe()
		callback("", fmt.Errorf("timeout"))
	}
	return nil
}

// Interface from BrokerConnectionInterface
func (c *NatsSupport) SetKey_P(key string) {
	c.key = key
}

// Interface from BrokerConnectionInterface
func (c *NatsSupport) GetKey_P() string {
	return c.key
}

// Interface from BrokerConnectionInterface
func (c *NatsSupport) GetBroker_P() any {
	return c
}
