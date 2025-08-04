package support

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"path/filepath"

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

func DownloadCAFile(endpoint, caFilePath, appID, appSecret string) error {
	// Ensure 'tls' directory exists
	dir := filepath.Dir(caFilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	// Check if file exists
	if _, err := os.Stat(caFilePath); err == nil {
		return nil // File exists, nothing to do
	}
	// Prepare JSON body
	body := map[string]string{
		"app_id":     appID,
		"app_secret": appSecret,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return err
	}
	// Download CA file
	resp, err := http.Post(endpoint, "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download CA file: status %d", resp.StatusCode)
	}
	out, err := os.Create(caFilePath)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
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
		// Build CA download endpoint from config endPoint
		caDownloadEndpoint := fmt.Sprintf("%s/api/worker/config/tls/download", Helper.ConfigYaml.ConfigData.End_point)
		appID := Helper.ConfigYaml.ConfigData.Credential.Project_id
		appSecret := Helper.ConfigYaml.ConfigData.Credential.Secret_key
		err := DownloadCAFile(caDownloadEndpoint, c.natConfInfo.CAFile, appID, appSecret)
		if err != nil {
			return fmt.Errorf("failed to download CA file: %v", err)
		}
		opts = append(opts, nats.RootCAs(c.natConfInfo.CAFile))
		fmt.Println("NATS TLS enabled. Using CA file:", c.natConfInfo.CAFile)
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
func (c *NatsSupport) SubSync(uuidItem string, group_id string, callback func(message string, err error), opts SubSyncOpts) error {
	unsubribce, err := c.nc.QueueSubscribeSync(uuidItem, group_id)
	if err != nil {
		log.Fatal(err)
		return err
	}
	messageCount := 0
	maxMessages := opts.Timeout_second // Maximum number of timeout process
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
func (c *NatsSupport) BasicSubSync(uuidItem string, callback func(message string, err error), opts SubSyncOpts) error {
	unsubribce, err := c.nc.SubscribeSync(uuidItem)
	if err != nil {
		log.Fatal(err)
		return err
	}

	var finish bool
	var errr error
	var msg *nats.Msg

	for !finish {
		msg, err = unsubribce.NextMsg(1 * time.Second) // Timeout after 5 seconds if no message
		if err != nil {
			if err == nats.ErrTimeout {
				fmt.Println("Timed out waiting for a message.")
				// messageCount++
			}
			continue
		}
		finish = true
	}
	// Process the received message
	fmt.Printf("\n unSubscribeFinish: %s\n", msg.Data)
	callback(string(msg.Data), nil)
	unsubribce.Unsubscribe()
	return errr
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
