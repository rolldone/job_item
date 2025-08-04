package support

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
		// Get endpoint, app_id, app_secret from global config
		caDownloadEndpoint := fmt.Sprintf("%s/api/worker/config/tls/download", Helper.ConfigYaml.ConfigData.End_point)
		appID := Helper.ConfigYaml.ConfigData.Credential.Project_id
		appSecret := Helper.ConfigYaml.ConfigData.Credential.Secret_key
		if config.CAFile != "" {
			certDir := filepath.Dir(config.CAFile)
			// Download and extract zip if CA file does not exist
			if _, err := os.Stat(config.CAFile); os.IsNotExist(err) {
				err := DownloadAndExtractRedisCerts(caDownloadEndpoint, certDir, appID, appSecret)
				if err != nil {
					return nil, err
				}
			}
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

func (r *RedisSupport) SubSync(uuidItem string, group string, callback func(message string, err error), opts SubSyncOpts) error {
	pubsub := r.client.Subscribe(context.Background(), uuidItem)
	defer pubsub.Close()
	select {
	case msg := <-pubsub.Channel():
		lockKey := fmt.Sprintf("lock:%s:%s:%x", uuidItem, group, msg.Payload)
		ok, err := r.client.SetNX(context.Background(), lockKey, "1", 5*time.Second).Result()
		if err != nil {
			callback("", fmt.Errorf("Redis lock error: %v", err))
			return err
		}
		if ok {
			callback(msg.Payload, nil)
			return nil
		} else {
			// Did not get lock, skip
			callback("", fmt.Errorf("lock not acquired"))
			return nil
		}
	case <-time.After(time.Duration(opts.Timeout_second) * time.Second):
		callback("", fmt.Errorf("timeout"))
		return nil
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

func (r *RedisSupport) BasicSubSync(topic string, callback func(message string, err error), opts SubSyncOpts) error {
	pubsub := r.client.Subscribe(context.Background(), topic)
	defer pubsub.Close()
	select {
	case msg := <-pubsub.Channel():
		callback(msg.Payload, nil)
		return nil
	case <-time.After(time.Duration(opts.Timeout_second) * time.Second):
		callback("", fmt.Errorf("timeout"))
		return nil
	}
}

// DownloadAndExtractRedisCerts downloads a zip file from the backend and extracts it to the given directory.
func DownloadAndExtractRedisCerts(endpoint, destDir, appID, appSecret string) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
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
	resp, err := http.Post(endpoint, "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download Redis certs zip: status %d", resp.StatusCode)
	}
	// Read zip into memory
	zipBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	zipReader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return err
	}
	for _, f := range zipReader.File {
		outPath := filepath.Join(destDir, f.Name)
		if f.FileInfo().IsDir() {
			os.MkdirAll(outPath, 0755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return err
		}
		outFile, err := os.Create(outPath)
		if err != nil {
			return err
		}
		inFile, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}
		_, err = io.Copy(outFile, inFile)
		inFile.Close()
		outFile.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// Interface from SupportInterface
func (r *RedisSupport) GetObject() any {
	return r
}
