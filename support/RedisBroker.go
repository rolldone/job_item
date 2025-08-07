package support

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisSupport struct {
	client *redis.Client
	key    string
}

func NewRedisSupportConstruct(config RedisBrokerConnection) (*RedisSupport, error) {
	var tlsConfig *tls.Config
	if config.Secure {
		if config.CAFile != "" {
			// Load CA cert
			caCert, err := os.ReadFile(config.CAFile)
			if err != nil {
				return nil, err
			}
			caCertPool := x509.NewCertPool()
			caCertPool.AppendCertsFromPEM(caCert)
			tlsConfig = &tls.Config{RootCAs: caCertPool}
			// If mTLS, load client cert/key
			if config.CertFile != "" && config.KeyFile != "" {
				cert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
				if err != nil {
					return nil, err
				}
				tlsConfig.Certificates = []tls.Certificate{cert}
			}
		}
	}
	client := redis.NewClient(&redis.Options{
		Addr:      fmt.Sprintf("%s:%d", config.Host, config.Port),
		Password:  config.Password, // no password set
		DB:        config.Db,       // use default DB
		TLSConfig: tlsConfig,
	})
	if err := client.Ping(context.Background()).Err(); err != nil {
		fmt.Println("Failed to connect to Redis:", err)
		return nil, err
	}
	return &RedisSupport{
		client: client,
		key:    config.Key,
	}, nil
}

func (r *RedisSupport) Pub(topic string, msg string) {
	r.client.Publish(context.Background(), topic, msg)
}

func (r *RedisSupport) Sub(uuidItem string, group string, callback func(message string)) (func(), error) {
	pubsub := r.client.Subscribe(context.Background(), uuidItem)
	go func() {
		for msg := range pubsub.Channel() {
			// Distributed lock key: topic + group + message ID
			// For simplicity, use message.Payload as ID (can be improved)
			lockKey := fmt.Sprintf("lock:%s:%s:%x", uuidItem, group, msg.Payload)
			// Try to acquire lock for this group
			ok, err := r.client.SetNX(context.Background(), lockKey, "1", 5*time.Second).Result()
			if err != nil {
				// Log error, skip processing
				fmt.Println("Redis lock error:", err)
				continue
			}
			if ok {
				// Got lock, process message
				callback(msg.Payload)
			} else {
				// Did not get lock, skip
				continue
			}
		}
	}()
	return func() { pubsub.Close() }, nil
}

func (r *RedisSupport) SubSync(uuidItem string, group string, callback func(message string, err error), opts SubSyncOpts) (bool, error) {
	pubsub := r.client.Subscribe(context.Background(), uuidItem)
	defer pubsub.Close()
	select {
	case msg := <-pubsub.Channel():
		lockKey := fmt.Sprintf("lock:%s:%s:%x", uuidItem, group, msg.Payload)
		ok, err := r.client.SetNX(context.Background(), lockKey, "1", 10*time.Second).Result()
		if err != nil {
			callback("", fmt.Errorf("Redis lock error: %v", err))
			return true, err
		}
		if ok {
			callback(msg.Payload, nil)
			return false, nil
		} else {
			// Did not get lock, skip
			callback("", fmt.Errorf("lock not acquired"))
			return true, nil
		}
	case <-time.After(time.Duration(opts.Timeout_second) * time.Second):
		callback("", fmt.Errorf("timeout"))
		return true, nil
	}
}

func (r *RedisSupport) GetBroker_P() any {
	return r
}

func (r *RedisSupport) SetKey_P(key string) {
	r.key = key
}

func (r *RedisSupport) GetKey_P() string {
	return r.key
}

func (r *RedisSupport) IsConnected() bool {
	return r.client != nil
}

func (c *RedisSupport) GetRefreshPubSub() string {
	return BROKER_REFRESH_PUBSUB
}

func (r *RedisSupport) BasicSub(topic string, callback func(message string)) (func(), error) {
	pubsub := r.client.Subscribe(context.Background(), topic)
	go func() {
		for msg := range pubsub.Channel() {
			callback(msg.Payload)
		}
	}()
	return func() { pubsub.Close() }, nil
}

func (r *RedisSupport) BasicSubSync(topic string, callback func(message string, err error), opts SubSyncOpts) (bool, error) {
	gg, err := r.SubSync(topic, "", callback, opts)
	return gg, err
}

// Interface from SupportInterface
func (r *RedisSupport) GetObject() any {
	return r
}
