package main

import (
	"errors"
	"job_item/support"
	"testing"
	"time"
)

func TestConfigSecondsEnv(t *testing.T) {
	t.Run("default when empty", func(t *testing.T) {
		t.Setenv("TEST_CONFIG_SECONDS", "")
		if got := configSecondsEnv("TEST_CONFIG_SECONDS", 7, "ERR-TEST"); got != 7*time.Second {
			t.Fatalf("got %s, want 7s", got)
		}
	})

	t.Run("custom value", func(t *testing.T) {
		t.Setenv("TEST_CONFIG_SECONDS", "3")
		if got := configSecondsEnv("TEST_CONFIG_SECONDS", 7, "ERR-TEST"); got != 3*time.Second {
			t.Fatalf("got %s, want 3s", got)
		}
	})

	t.Run("invalid value falls back", func(t *testing.T) {
		t.Setenv("TEST_CONFIG_SECONDS", "abc")
		if got := configSecondsEnv("TEST_CONFIG_SECONDS", 7, "ERR-TEST"); got != 7*time.Second {
			t.Fatalf("got %s, want 7s", got)
		}
	})

	t.Run("non-positive value falls back", func(t *testing.T) {
		t.Setenv("TEST_CONFIG_SECONDS", "0")
		if got := configSecondsEnv("TEST_CONFIG_SECONDS", 7, "ERR-TEST"); got != 7*time.Second {
			t.Fatalf("got %s, want 7s", got)
		}
	})
}

func TestNextBackoffRemoved(t *testing.T) {
	// Retry delays are fixed now; this guards against reintroducing exponential growth.
	t.Setenv("JOB_ITEM_CONFIG_FETCH_BACKOFF_SECONDS", "2")
	t.Setenv("JOB_ITEM_CONFIG_FETCH_MAX_INTERVAL_SECONDS", "300")
	t.Setenv("JOB_ITEM_CONFIG_FETCH_MIN_GAP_SECONDS", "30")

	opts := loadConfigFetchOptions()
	if opts.retryInterval != 2*time.Second {
		t.Fatalf("retryInterval = %s, want the fixed 2s value", opts.retryInterval)
	}
}

func TestWithJitterWithinRange(t *testing.T) {
	const base = 10 * time.Second
	for i := 0; i < 200; i++ {
		got := withJitter(base)
		if got < 8*time.Second || got > 12*time.Second {
			t.Fatalf("withJitter(%s) = %s, want within [8s, 12s]", base, got)
		}
	}
}

func TestLoadConfigFetchOptionsUsesFixedRetryInterval(t *testing.T) {
	t.Setenv("JOB_ITEM_CONFIG_FETCH_BACKOFF_SECONDS", "7")
	t.Setenv("JOB_ITEM_CONFIG_FETCH_MAX_INTERVAL_SECONDS", "300")
	t.Setenv("JOB_ITEM_CONFIG_FETCH_MIN_GAP_SECONDS", "5")

	opts := loadConfigFetchOptions()
	if opts.retryInterval != 7*time.Second {
		t.Fatalf("retryInterval = %s, want 7s", opts.retryInterval)
	}
	if opts.minGap != 5*time.Second {
		t.Fatalf("minGap = %s, want 5s", opts.minGap)
	}
}

func TestLoadConfigFetchOptionsClampsRetryInterval(t *testing.T) {
	t.Setenv("JOB_ITEM_CONFIG_FETCH_BACKOFF_SECONDS", "600")
	t.Setenv("JOB_ITEM_CONFIG_FETCH_MAX_INTERVAL_SECONDS", "300")
	t.Setenv("JOB_ITEM_CONFIG_FETCH_MIN_GAP_SECONDS", "30")

	opts := loadConfigFetchOptions()
	if opts.retryInterval != 300*time.Second {
		t.Fatalf("retryInterval = %s, want 300s (clamped to max interval)", opts.retryInterval)
	}
}

func fastRetryOpts() configFetchOptions {
	return configFetchOptions{
		retryInterval: time.Millisecond,
		maxInterval:   2 * time.Millisecond,
		minGap:        0,
	}
}

func TestRetryWorkerStartupRetriesUntilSuccess(t *testing.T) {
	attempts := 0
	err := retryWorkerStartup(func() error {
		attempts++
		if attempts < 5 {
			return errors.New("broker config missing")
		}
		return nil
	}, fastRetryOpts(), 0)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 5 {
		t.Fatalf("attempts = %d, want 5", attempts)
	}
}

func TestRetryWorkerStartupStopsOnRefusedFetch(t *testing.T) {
	attempts := 0
	err := retryWorkerStartup(func() error {
		attempts++
		return support.ErrConfigFetchRefused
	}, fastRetryOpts(), 0)

	if !errors.Is(err, support.ErrConfigFetchRefused) {
		t.Fatalf("err = %v, want ErrConfigFetchRefused", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1 (must not retry refused fetch)", attempts)
	}
}

func TestRetryWorkerStartupHonorsMaxAttempts(t *testing.T) {
	attempts := 0
	err := retryWorkerStartup(func() error {
		attempts++
		return errors.New("still failing")
	}, fastRetryOpts(), 3)

	if err == nil {
		t.Fatal("expected error after max attempts")
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}
