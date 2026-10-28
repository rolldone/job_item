package support

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeTestCACert writes a self-signed CA certificate in PEM format and returns its path.
func writeTestCACert(t *testing.T) string {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "amqp-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatalf("failed to write CA file: %v", err)
	}
	return path
}

func TestBuildTLSConfig_SecureWithoutCAFile(t *testing.T) {
	support := AMQPSupport{
		amqpConfInfo: AMQP_BrokerConnection{Secure: true},
	}

	tlsConfig, err := support.buildTLSConfig()
	if err != nil {
		t.Fatalf("expected no error for secure without ca_file, got: %v", err)
	}
	if tlsConfig == nil {
		t.Fatal("expected tls config, got nil")
	}
	if tlsConfig.RootCAs != nil {
		t.Fatal("expected RootCAs to be nil so system roots are used")
	}
}

func TestBuildTLSConfig_SecureWithCAFile(t *testing.T) {
	caPath := writeTestCACert(t)
	support := AMQPSupport{
		amqpConfInfo: AMQP_BrokerConnection{Secure: true, CAFile: caPath},
	}

	tlsConfig, err := support.buildTLSConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tlsConfig == nil || tlsConfig.RootCAs == nil {
		t.Fatal("expected RootCAs populated from ca_file")
	}
}

func TestBuildTLSConfig_SecureWithMissingCAFile(t *testing.T) {
	support := AMQPSupport{
		amqpConfInfo: AMQP_BrokerConnection{
			Secure: true,
			CAFile: filepath.Join(t.TempDir(), "missing.pem"),
		},
	}

	if _, err := support.buildTLSConfig(); err == nil {
		t.Fatal("expected error for missing ca_file, got nil")
	}
}

func TestBuildTLSConfig_SecureWithInvalidCAFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.pem")
	if err := os.WriteFile(path, []byte("not a pem"), 0o600); err != nil {
		t.Fatalf("failed to write invalid CA file: %v", err)
	}
	support := AMQPSupport{
		amqpConfInfo: AMQP_BrokerConnection{Secure: true, CAFile: path},
	}

	if _, err := support.buildTLSConfig(); err == nil {
		t.Fatal("expected error for unparsable ca_file, got nil")
	}
}

func TestBuildTLSConfig_Insecure(t *testing.T) {
	support := AMQPSupport{
		amqpConfInfo: AMQP_BrokerConnection{Secure: false, CAFile: "/does/not/exist"},
	}

	tlsConfig, err := support.buildTLSConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tlsConfig != nil {
		t.Fatal("expected nil tls config for insecure connection")
	}
}

func TestBuildTLSConfig_ClientCertOnlyWhenBothProvided(t *testing.T) {
	caPath := writeTestCACert(t)
	support := AMQPSupport{
		amqpConfInfo: AMQP_BrokerConnection{Secure: true, CAFile: caPath, CertFile: "/only/cert.pem"},
	}

	tlsConfig, err := support.buildTLSConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tlsConfig.Certificates) != 0 {
		t.Fatal("expected no client certificate when key_file is missing")
	}
}
