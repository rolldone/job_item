package main

import (
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

func TestNextBackoff(t *testing.T) {
	tests := []struct {
		current time.Duration
		max     time.Duration
		want    time.Duration
	}{
		{current: 2 * time.Second, max: 5 * time.Minute, want: 4 * time.Second},
		{current: 4 * time.Minute, max: 5 * time.Minute, want: 5 * time.Minute},
		{current: 5 * time.Minute, max: 5 * time.Minute, want: 5 * time.Minute},
	}

	for _, tt := range tests {
		if got := nextBackoff(tt.current, tt.max); got != tt.want {
			t.Fatalf("nextBackoff(%s, %s) = %s, want %s", tt.current, tt.max, got, tt.want)
		}
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

func TestLoadConfigFetchOptionsClampsCeiling(t *testing.T) {
	t.Setenv("JOB_ITEM_CONFIG_FETCH_BACKOFF_SECONDS", "60")
	t.Setenv("JOB_ITEM_CONFIG_FETCH_MAX_INTERVAL_SECONDS", "10")
	t.Setenv("JOB_ITEM_CONFIG_FETCH_MIN_GAP_SECONDS", "5")

	opts := loadConfigFetchOptions()
	if opts.initialBackoff != 60*time.Second {
		t.Fatalf("initialBackoff = %s, want 60s", opts.initialBackoff)
	}
	if opts.maxInterval != 60*time.Second {
		t.Fatalf("maxInterval = %s, want 60s (clamped to initial backoff)", opts.maxInterval)
	}
	if opts.minGap != 5*time.Second {
		t.Fatalf("minGap = %s, want 5s", opts.minGap)
	}
}
