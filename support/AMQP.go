package support

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type AMQPConfInfo struct {
	AMQP_HOST     string
	AMQP_PORT     int
	AMQP_USER     string
	AMQP_PASSWORD string
	AMQP_EXCHANGE string
}

func AMQPSupportConstruct(props AMQP_BrokerConnection) (*AMQPSupport, error) {
	gg := AMQPSupport{
		amqpConfInfo: props,
	}
	_, err := gg.ConnectPubSub()
	return &gg, err
}

type AMQPSupport struct {
	ch           *amqp.Channel
	nc           *amqp.Connection
	amqpConfInfo AMQP_BrokerConnection
	key          string
}

func (c *AMQPSupport) GetRefreshPubSub() string {
	return BROKER_REFRESH_PUBSUB
}

func (c *AMQPSupport) ConnectPubSub() (*amqp.Connection, error) {
	amqpHost := c.amqpConfInfo.Host
	amqpPort := c.amqpConfInfo.Port
	amqpUser := c.amqpConfInfo.User
	amqpPassword := c.amqpConfInfo.Password

	// Create AMQP URI
	url := "amqp://" + amqpUser + ":" + amqpPassword + "@" + amqpHost + ":" + strconv.Itoa(amqpPort) + "/"
	if c.amqpConfInfo.Secure {
		url = "amqps://" + amqpUser + ":" + amqpPassword + "@" + amqpHost + ":" + strconv.Itoa(amqpPort) + "/"
	}
	fmt.Println("AMQP Connection inf :: ", url)

	// Perform a single connection attempt. Startup retries are bounded by the
	// caller (initBrokerConnections / tryRestartProcess) so the worker can never
	// hang forever when the broker is unreachable.
	nc, err := c.dialAMQP(url)
	if err != nil {
		return nil, fmt.Errorf("AMQP connection to %s failed: %w", url, err)
	}
	c.nc = nc

	// Set up connection monitoring
	go func() {
		notifyClose := nc.NotifyClose(make(chan *amqp.Error))
		for err := range notifyClose {
			if err != nil {
				fmt.Println("Disconnected from AMQP server, attempting to reconnect:", err)
				go c.retryConnection(url) // Trigger reconnection loop
			}
		}
	}()

	// Create a channel
	ch, chErr := nc.Channel()
	if chErr != nil {
		fmt.Println("Failed to open a channel:", chErr)
		return nil, chErr
	}
	c.ch = ch

	return c.nc, nil
}

func (c *AMQPSupport) dialAMQP(url string) (*amqp.Connection, error) {
	tlsConfig, err := c.buildTLSConfig()
	if err != nil {
		return nil, err
	}
	return amqp.DialConfig(url, buildAMQPConfig(tlsConfig))
}

// Values mirror amqp091-go's unexported defaults (connection.go) so that using
// DialConfig keeps the same tuning as the previous DialTLS/Dial calls.
const (
	defaultAMQPHeartbeat   = 10 * time.Second
	defaultAMQPLocale      = "en_US"
	defaultAMQPDialTimeout = 10 * time.Second
)

// buildAMQPConfig builds the AMQP client config.
// Heartbeat and Locale are set explicitly because amqp091-go only applies its
// defaults inside Dial/DialTLS/DialConfig when the config is zero-valued.
// Leaving Heartbeat at 0 makes the server interval win (RabbitMQ default 60s),
// which delays dead-connection detection by ~3x.
func buildAMQPConfig(tlsConfig *tls.Config) amqp.Config {
	return amqp.Config{
		Dial:            amqp.DefaultDial(defaultAMQPDialTimeout),
		Heartbeat:       defaultAMQPHeartbeat,
		Locale:          defaultAMQPLocale,
		TLSClientConfig: tlsConfig,
	}
}

// buildTLSConfig builds the TLS configuration for a secure AMQP connection.
// A missing ca_file keeps the previous behaviour: the client falls back to the
// system root CAs instead of failing. When ca_file is set it is used as the
// only root CA pool. Client cert/key are only loaded when both are provided.
func (c *AMQPSupport) buildTLSConfig() (*tls.Config, error) {
	if !c.amqpConfInfo.Secure {
		return nil, nil
	}

	tlsConfig := &tls.Config{}
	if c.amqpConfInfo.CAFile != "" {
		caCert, err := os.ReadFile(c.amqpConfInfo.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA file %q: %w", c.amqpConfInfo.CAFile, err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA file %q: no certificates found", c.amqpConfInfo.CAFile)
		}
		tlsConfig.RootCAs = caCertPool
	}
	if c.amqpConfInfo.CertFile != "" && c.amqpConfInfo.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(c.amqpConfInfo.CertFile, c.amqpConfInfo.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client cert/key: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	return tlsConfig, nil
}

func (c *AMQPSupport) retryConnection(url string) {
	for {
		nc, connErr := c.dialAMQP(url)
		if connErr == nil {
			c.nc = nc
			fmt.Println("Successfully reconnected to AMQP server:", url)

			// Recreate the channel
			ch, chErr := nc.Channel()
			if chErr != nil {
				fmt.Println("Failed to reopen channel:", chErr)
				go c.retryConnection(url) // Recursive retry on channel failure
				return
			}
			c.ch = ch

			// Set up connection monitoring
			go func() {
				notifyClose := nc.NotifyClose(make(chan *amqp.Error))
				for err := range notifyClose {
					if err != nil {
						fmt.Println("Disconnected from AMQP server, attempting to reconnect:", err)
						go c.retryConnection(url) // Recursive retry on disconnect
					}
				}
			}()

			// Publish an event to refresh pubsub
			Helper.EventBus.GetBus().Publish("refresh_pubsub", nil)
			break
		}

		// Log and retry after a delay
		fmt.Println("Retrying AMQP connection in 5-10 seconds:", connErr.Error())
		time.Sleep(5 * time.Second)
	}
}

// Interface from SupportInterface
func (c *AMQPSupport) GetObject() any {
	return c
}

// Interface from BrokerConnectionInterface
func (c *AMQPSupport) Pub(topic string, msg string) {
	// c.ch.Publish(topic, []byte(msg))
	// Ensure the channel is open
	if c.ch == nil {
		log.Println("channel is not open")
		return
	}

	// Ensure the connection is open
	if c.nc == nil {
		log.Println("connection is not open")
		return

	}
	// Create a context for the publish operation
	ctx := context.Background()
	// Publish the message
	err := c.ch.PublishWithContext(
		ctx,
		"",    // exchange
		topic, // routing key (queue name)
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			ContentType: "text/plain",
			Body:        []byte(msg),
		},
	)
	if err != nil {
		// if err == amqp.ErrClosed {
		// 	// Reopen the channel
		// 	c.ch, err = c.nc.Channel()
		// 	if err != nil {
		// 		log.Println(fmt.Errorf("failed to reopen channel: %w", err))
		// 		return
		// 	}
		// 	// Retry message publishing
		// 	// return c.Publish(topic, group_id, message)
		// }
		log.Println(fmt.Errorf("failed to publish message: %w", err))
		return
	}

}

// Interface from BrokerConnectionInterface
func (c *AMQPSupport) Sub(topic string, group_id string, callback func(message string)) (func(), error) {

	key_topic := topic

	_, err := c.ch.QueueDeclare(key_topic, false, true, false, false, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to declare queue: %w", err)
	}
	msgs, err := c.ch.Consume(key_topic, "", true, false, false, false, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to register consumer: %w", err)
	}
	go func() {
		for msg := range msgs {
			log.Printf("Received a message: %s", msg.Body)
			callback(string(msg.Body))
			// Process the message here
		}
	}()
	// Return a closure to cancel the consumer
	cancel := func() {
		log.Println("Sub AMQPSupport :: ", key_topic, " :: Closing")
		if _, err := c.ch.QueueDelete(key_topic, false, false, false); err != nil {
			log.Printf("Failed to cancel consumer: %v", err)
		}
		log.Println("Sub AMQPSupport :: ", key_topic, " :: Closed")
	}
	return cancel, nil
}

// Interface from BrokerConnectionInterface
// For this message queue case we ignore group_id
func (c *AMQPSupport) SubSync(uuidItem string, group_id string, callback func(message string, err error), opts SubSyncOpts) (bool, error) {
	key_topic := uuidItem

	_, err := c.ch.QueueDeclare(key_topic, false, true, false, false, nil)
	if err != nil {
		return true, fmt.Errorf("failed to declare queue: %w", err)
	}
	msgs, err := c.ch.Consume(key_topic, "", true, false, false, false, nil)
	if err != nil {
		return true, fmt.Errorf("failed to register consumer: %w", err)
	}

	timeout := time.After(time.Duration(opts.Timeout_second) * time.Second)
	var errr error

	select {
	case msg, ok := <-msgs:
		if !ok {
			errr = errors.New("channel closed")
			callback(GetStatus().STATUS_ERROR, errr)
			return true, errr
		}
		log.Printf("unSubscribeFinish: %s\n", msg.Body)
		if _, errr = c.ch.QueueDelete(key_topic, false, false, false); errr != nil {
			log.Printf("Failed to cancel consumer: %v", errr)
		}
		callback(string(msg.Body), nil)
		return false, nil // Not timeout
	case <-timeout:
		if _, errr = c.ch.QueueDelete(key_topic, false, false, false); errr != nil {
			log.Printf("Failed to cancel consumer: %v", errr)
		}
		return true, nil // Timeout
	}
}

// Interface from BrokerConnectionInterface
func (c *AMQPSupport) BasicSub(topic string, callback func(message string)) (func(), error) {
	key_topic := topic
	// if group_id == "" {
	// 	key_topic = topic
	// }
	_, err := c.ch.QueueDeclare(key_topic, false, true, false, false, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to declare queue: %w", err)
	}
	msgs, err := c.ch.Consume(key_topic, "", true, false, false, false, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to register consumer: %w", err)
	}
	go func() {
		for msg := range msgs {
			log.Printf("Received a message: %s", msg.Body)
			callback(string(msg.Body))
			// Process the message here
		}
	}()
	// Return a closure to cancel the consumer
	cancel := func() {
		log.Println("Sub AMQPSupport :: ", key_topic, " :: Closing")
		if _, err := c.ch.QueueDelete(key_topic, false, false, false); err != nil {
			log.Printf("Failed to cancel consumer: %v", err)
		}
		log.Println("Sub AMQPSupport :: ", key_topic, " :: Closed")
	}
	return cancel, nil
}

// Interface from BrokerConnectionInterface
func (c *AMQPSupport) BasicSubSync(uuidItem string, callback func(message string, err error), opts SubSyncOpts) (bool, error) {
	key_topic := uuidItem

	_, err := c.ch.QueueDeclare(key_topic, false, true, false, false, nil)
	if err != nil {
		return true, fmt.Errorf("failed to declare queue: %w", err)
	}
	msgs, err := c.ch.Consume(key_topic, "", true, false, false, false, nil)
	if err != nil {
		return true, fmt.Errorf("failed to register consumer: %w", err)
	}
	timeout := time.After(time.Duration(opts.Timeout_second) * time.Second)
	var errr error

	select {
	case msg, ok := <-msgs:
		if !ok {
			errr = errors.New("channel closed")
			callback(GetStatus().STATUS_ERROR, errr)
			return true, errr
		}
		log.Printf("unSubscribeFinish: %s\n", msg.Body)
		if _, errr = c.ch.QueueDelete(key_topic, false, false, false); errr != nil {
			log.Printf("Failed to cancel consumer: %v", errr)
		}
		callback(string(msg.Body), nil)
		return false, nil // Not timeout
	case <-timeout:
		if _, errr = c.ch.QueueDelete(key_topic, false, false, false); errr != nil {
			log.Printf("Failed to cancel consumer: %v", errr)
		}
		return true, nil // Timeout
	}
}

// Interface from BrokerConnectionInterface
func (c *AMQPSupport) SetKey_P(key string) {
	c.key = key
}

// Interface from BrokerConnectionInterface
func (c *AMQPSupport) GetKey_P() string {
	return c.key
}

// Interface from BrokerConnectionInterface
func (c *AMQPSupport) GetBroker_P() any {
	return c
}

// Interface from BrokerConnectionInterface
func (c *AMQPSupport) IsConnected() bool {
	if c.nc == nil || c.nc.IsClosed() {
		return false
	}
	return true
}
