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

func NatsSupportConstruct(props NatsBrokerConnection) (NatsSupport, error) {
	gg := NatsSupport{
		natConfInfo: props,
	}
	err := gg.ConnectPubSub()
	return gg, err
}

func (c *NatsSupport) GetRefreshPubSub() string {
	return BROKER_REFRESH_PUBSUB
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

	// Build connection options
	var opts []nats.Option
	opts = append(opts,
		nats.MaxReconnects(-1),
		nats.ReconnectWait(5*time.Second),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			fmt.Println("Reconnected to NATS server:", nc.ConnectedUrl())
		}),
		nats.DisconnectHandler(func(nc *nats.Conn) {
			fmt.Println("Disconnected from NATS server, attempting to reconnect...")
		}),
		nats.ClosedHandler(func(nc *nats.Conn) {
			fmt.Println("Connection to NATS server closed, attempting to reconnect...")
		}),
	)
	// Add TLS option if Secure is true
	if c.natConfInfo.Secure {
		if c.natConfInfo.CAFile == "" {
			return fmt.Errorf("NATS TLS enabled but ca_file is missing in config")
		}
		opts = append(opts, nats.RootCAs(c.natConfInfo.CAFile))
		fmt.Println("NATS TLS enabled. Using CA file:", c.natConfInfo.CAFile)
	}
	if c.natConfInfo.CertFile != "" && c.natConfInfo.KeyFile != "" {
		opts = append(opts, nats.ClientCert(c.natConfInfo.CertFile, c.natConfInfo.KeyFile))
		fmt.Println("NATS mTLS enabled. Using client cert and key:", c.natConfInfo.CertFile, c.natConfInfo.KeyFile)
	}

	// Retry mechanism
	var err error
	for {
		nc, connErr := nats.Connect(url, opts...)
		if connErr == nil {
			c.nc = nc
			err = connErr
			break
		}
		fmt.Println("Nats connection failed, retrying in 5-10 seconds:", connErr.Error())
		time.Sleep(5 * time.Second) // Delay before retrying
		err = connErr
	}
	fmt.Println("Successfully reconnected to NATS server - main")
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
// Improved: Waits for a message up to the specified duration. Returns (isTimeout, error).
func (c *NatsSupport) SubSync(uuidItem string, group_id string, callback func(message string, err error), opts SubSyncOpts) (bool, error) {
	unsubribce, err := c.nc.QueueSubscribeSync(uuidItem, group_id)
	if err != nil {
		log.Fatal(err)
		return true, err
	}

	// Calculate deadline
	deadline := time.Now().Add(time.Duration(opts.Timeout_second) * time.Second)
	var msg *nats.Msg

	for time.Now().Before(deadline) {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		msg, err = unsubribce.NextMsg(remaining)
		if err != nil {
			if err == nats.ErrTimeout {
				// Timed out waiting for a message in this interval, but check total deadline
				continue
			}
			unsubribce.Unsubscribe()
			return true, err // Some other error
		}
		// Got a message
		fmt.Printf("\n unSubscribeFinish: %s\n", msg.Data)
		callback(string(msg.Data), nil)
		unsubribce.Unsubscribe()
		return false, nil
	}
	// If we reach here, timeout occurred
	unsubribce.Unsubscribe()
	return true, nil
}

// Interface from BrokerConnectionInterface
func (c *NatsSupport) BasicSub(topic string, callback func(message string)) (func(), error) {
	unsubribce, err := c.nc.Subscribe(topic, func(msg *nats.Msg) {
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
func (c *NatsSupport) BasicSubSync(uuidItem string, callback func(message string, err error), opts SubSyncOpts) (bool, error) {
	unsubribce, err := c.nc.SubscribeSync(uuidItem)
	if err != nil {
		log.Fatal(err)
		return true, err
	}

	// Calculate deadline
	deadline := time.Now().Add(time.Duration(opts.Timeout_second) * time.Second)
	var msg *nats.Msg

	for time.Now().Before(deadline) {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		msg, err = unsubribce.NextMsg(remaining)
		if err != nil {
			if err == nats.ErrTimeout {
				// Timed out waiting for a message in this interval, but check total deadline
				continue
			}
			unsubribce.Unsubscribe()
			return true, err // Some other error
		}
		// Got a message
		fmt.Printf("\n unSubscribeFinish: %s\n", msg.Data)
		callback(string(msg.Data), nil)
		unsubribce.Unsubscribe()
		return false, nil
	}
	// If we reach here, timeout occurred
	unsubribce.Unsubscribe()
	return true, nil
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

// Interface from BrokerConnectionInterface
func (c *NatsSupport) IsConnected() bool {
	if c.nc == nil || c.nc.Status() != nats.CONNECTED {
		return false
	}
	return true
}
