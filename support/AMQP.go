package support

import (
	"context"
	"errors"
	"fmt"
	"log"
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

func (c *AMQPSupport) ConnectPubSub() (*amqp.Connection, error) {
	amqpHost := c.amqpConfInfo.Host
	amqpPort := c.amqpConfInfo.Port
	amqpUser := c.amqpConfInfo.User
	amqpPassword := c.amqpConfInfo.Password

	// Create AMQP URI
	url := "amqp://" + amqpUser + ":" + amqpPassword + "@" + amqpHost + ":" + strconv.Itoa(amqpPort) + "/"
	fmt.Println("AMQP Connection inf :: ", url)

	// Retry mechanism
	var err error
	for {
		nc, connErr := amqp.Dial(url)
		if connErr == nil {
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
			break
		}
		fmt.Println("AMQP connection failed, retrying in 5-10 seconds:", connErr.Error())
		time.Sleep(5 * time.Second) // Delay before retrying
		err = connErr
	}

	return c.nc, err
}

func (c *AMQPSupport) retryConnection(url string) {
	for {
		// Check if already connected
		if c.IsConnected() {
			fmt.Println("Already connected to AMQP server, skipping reconnection")
			return
		}

		// Attempt to reconnect
		nc, connErr := amqp.Dial(url)
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
func (c *AMQPSupport) SubSync(uuidItem string, group_id string, callback func(message string, err error), opts SubSyncOpts) error {

	key_topic := group_id + "." + uuidItem
	if group_id == "" {
		key_topic = uuidItem
	}
	_, err := c.ch.QueueDeclare(key_topic, false, true, false, false, nil)
	if err != nil {
		return fmt.Errorf("failed to declare queue: %w", err)
	}
	msgs, err := c.ch.Consume(key_topic, "", true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("failed to register consumer: %w", err)
	}

	// messageCount := 0
	maxMessages := opts.Timeout // Maximum number of timeout process
	// Use a timeout duration of 10 seconds
	timeout := time.After(time.Duration(maxMessages) * time.Second) // Adjust timeout duration as needed
	var finish bool
	var errr error
	for !finish {
		select {
		case msg, ok := <-msgs:
			// Process the message here
			if !ok {
				errr = errors.New("channel closed")
				callback("", errr)
				finish = true
				break
			}
			// Process the received message
			log.Printf("unSubscribeFinish: %s\n", msg.Body)
			if _, errr = c.ch.QueueDelete(group_id+"."+uuidItem, false, false, false); errr != nil {
				log.Printf("Failed to cancel consumer: %v", errr)
			}
			callback(string(msg.Body), nil)
			finish = true
		case <-timeout:
			log.Println("Timeout reached. Stopping message consumption.")
			if _, errr = c.ch.QueueDelete(group_id+"."+uuidItem, false, false, false); errr != nil {
				log.Printf("Failed to cancel consumer: %v", errr)
			} else {
				callback("", fmt.Errorf("timeout"))
			}
			finish = true
		}
	}
	return errr
	// for messageCount < maxMessages {
	// 	// Wait for a message
	// 	for
	// 	msg, err := unsubribce.NextMsg(1 * time.Second) // Timeout after 5 seconds if no message
	// 	if err != nil {
	// 		if err == amqp.ErrTimeout {
	// 			fmt.Println("Timed out waiting for a message.", (messageCount + 1))
	// 			messageCount++
	// 			continue
	// 		}
	// 		log.Fatal(err)
	// 	}

	// 	// Process the received message
	// 	fmt.Printf("\n unSubscribeFinish: %s\n", msg.Data)
	// 	callback(string(msg.Data), nil)
	// 	unsubribce.Unsubscribe()
	// 	break
	// }
	// if messageCount >= maxMessages {
	// 	unsubribce.Unsubscribe()
	// 	callback(GetStatus().STATUS_TIMEOUT, nil)
	// }
	// return nil
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
