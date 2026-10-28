package main

import (
	"errors"
	"job_item/support"
	"os"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	support.SupportConstruct("test")
	os.Exit(m.Run())
}

func TestTryRestartProcess_StopsAtMaxAttempts(t *testing.T) {
	attempts := 0
	errRetry := errors.New("broker unreachable")

	err := tryRestartProcess("broker-key", "nats", "identity-1", 3, time.Millisecond, func() error {
		attempts++
		return errRetry
	})

	if err == nil {
		t.Fatal("expected error after exhausting attempts, got nil")
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
	if !errors.Is(err, errRetry) {
		t.Fatalf("expected wrapped last error, got %v", err)
	}
}

func TestTryRestartProcess_SucceedsAfterRetries(t *testing.T) {
	attempts := 0

	err := tryRestartProcess("broker-key", "redis", "identity-1", 5, time.Millisecond, func() error {
		attempts++
		if attempts < 3 {
			return errors.New("not ready")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected success on attempt 3, got %d attempts", attempts)
	}
}

func TestTryRestartProcess_SucceedsFirstAttempt(t *testing.T) {
	attempts := 0

	err := tryRestartProcess("broker-key", "rabbitmq", "identity-1", 5, time.Millisecond, func() error {
		attempts++
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", attempts)
	}
}

func TestBrokerConnectMaxAttempts(t *testing.T) {
	t.Run("default when empty", func(t *testing.T) {
		t.Setenv("JOB_ITEM_BROKER_CONNECT_MAX_ATTEMPTS", "")
		if got := brokerConnectMaxAttempts(); got != defaultBrokerConnectMaxAttempts {
			t.Fatalf("got %d, want %d", got, defaultBrokerConnectMaxAttempts)
		}
	})

	t.Run("custom value", func(t *testing.T) {
		t.Setenv("JOB_ITEM_BROKER_CONNECT_MAX_ATTEMPTS", "7")
		if got := brokerConnectMaxAttempts(); got != 7 {
			t.Fatalf("got %d, want 7", got)
		}
	})

	t.Run("invalid value falls back to default", func(t *testing.T) {
		t.Setenv("JOB_ITEM_BROKER_CONNECT_MAX_ATTEMPTS", "not-a-number")
		if got := brokerConnectMaxAttempts(); got != defaultBrokerConnectMaxAttempts {
			t.Fatalf("got %d, want %d", got, defaultBrokerConnectMaxAttempts)
		}
	})

	t.Run("non-positive value falls back to default", func(t *testing.T) {
		t.Setenv("JOB_ITEM_BROKER_CONNECT_MAX_ATTEMPTS", "0")
		if got := brokerConnectMaxAttempts(); got != defaultBrokerConnectMaxAttempts {
			t.Fatalf("got %d, want %d", got, defaultBrokerConnectMaxAttempts)
		}
	})
}

func TestBrokerConnectBackoff(t *testing.T) {
	t.Run("default when empty", func(t *testing.T) {
		t.Setenv("JOB_ITEM_BROKER_CONNECT_BACKOFF_SECONDS", "")
		if got := brokerConnectBackoff(); got != defaultBrokerConnectBackoff {
			t.Fatalf("got %s, want %s", got, defaultBrokerConnectBackoff)
		}
	})

	t.Run("custom value", func(t *testing.T) {
		t.Setenv("JOB_ITEM_BROKER_CONNECT_BACKOFF_SECONDS", "2")
		if got := brokerConnectBackoff(); got != 2*time.Second {
			t.Fatalf("got %s, want 2s", got)
		}
	})

	t.Run("invalid value falls back to default", func(t *testing.T) {
		t.Setenv("JOB_ITEM_BROKER_CONNECT_BACKOFF_SECONDS", "abc")
		if got := brokerConnectBackoff(); got != defaultBrokerConnectBackoff {
			t.Fatalf("got %s, want %s", got, defaultBrokerConnectBackoff)
		}
	})
}
