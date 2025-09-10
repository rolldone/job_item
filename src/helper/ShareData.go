package helper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// ----- in-memory share data helper -----
// ShareDataOptions configures behavior for a task's share data.
type ShareDataOptions struct {
	// DefaultTTLSeconds applied to added entries when ttl is not supplied
	DefaultTTLSeconds int
}

type shareEntry struct {
	Data      string
	CreatedAt time.Time
	ExpiresAt time.Time
}

var (
	// store[taskID][key] => slice of entries
	storeMu sync.RWMutex
	store   = map[string]map[string][]shareEntry{}
	// options per task
	optsMu  sync.RWMutex
	options = map[string]ShareDataOptions{}
)

// ShareDataOption sets options for a task (e.g., default TTL seconds).
func ShareDataOption(taskID string, o ShareDataOptions) {
	optsMu.Lock()
	options[taskID] = o
	optsMu.Unlock()
}

// ShareDataAdd appends data to key under a task. If ttlSeconds <=0 the task default is used (may be 0 = no expiry).
func ShareDataAdd(taskID, key, data string, ttlSeconds int) {
	// If JOB_ITEM_SHARE_DATA_ADD is configured, forward the add to that endpoint.
	addURL := os.Getenv("JOB_ITEM_SHARE_DATA_ADD")
	if addURL != "" {
		// // support :task_id placeholder or append taskID to path
		// if strings.Contains(addURL, ":task_id") {
		// 	addURL = strings.ReplaceAll(addURL, ":task_id", taskID)
		// } else {
		// 	if strings.HasSuffix(addURL, "/") {
		// 		addURL = addURL + taskID
		// 	} else {
		// 		addURL = addURL + "/" + taskID
		// 	}
		// }

		// build payload
		payload := map[string]interface{}{
			"key":  key,
			"data": data,
		}
		if ttlSeconds > 0 {
			payload["ttl"] = ttlSeconds
		} else {
			// if ttlSeconds <= 0, include default ttl if configured
			optsMu.RLock()
			if to, ok := options[taskID]; ok && to.DefaultTTLSeconds > 0 {
				payload["ttl"] = to.DefaultTTLSeconds
			}
			optsMu.RUnlock()
		}
		fmt.Println("-----------------addURL :: ", addURL)
		body, _ := json.Marshal(payload)
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Post(addURL, "application/json", bytes.NewReader(body))
		fmt.Println("-----------------resp :: ", resp, " err :: ", err)
		if err != nil {
			fmt.Println("ShareDataAdd: remote post error, falling back to local store:", err)
			// fallthrough to local store
		} else {
			defer resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				// successfully forwarded, do not store locally
				return
			}
			fmt.Println("ShareDataAdd: remote returned status", resp.StatusCode, "- falling back to local store")
			// fallthrough to local store
		}
		return
	}

	fmt.Println("JOB_ITEM_SHARE_DATA_ADD :: ", addURL)
	// determine expiry for local store
	var expiresAt time.Time
	if ttlSeconds <= 0 {
		optsMu.RLock()
		if to, ok := options[taskID]; ok && to.DefaultTTLSeconds > 0 {
			expiresAt = time.Now().Add(time.Duration(to.DefaultTTLSeconds) * time.Second)
		}
		optsMu.RUnlock()
	} else {
		expiresAt = time.Now().Add(time.Duration(ttlSeconds) * time.Second)
	}

	entry := shareEntry{Data: data, CreatedAt: time.Now()}
	if !expiresAt.IsZero() {
		entry.ExpiresAt = expiresAt
	}

	storeMu.Lock()
	defer storeMu.Unlock()
	if _, ok := store[taskID]; !ok {
		store[taskID] = map[string][]shareEntry{}
	}
	store[taskID][key] = append(store[taskID][key], entry)
	fmt.Println("store after add :: ", store[taskID][key])
}

// ShareDataGet returns all non-expired data strings for a task/key in insertion order.
// ShareDataGet returns all non-expired data strings for a task/key in insertion order.
// If nothing is found in-memory it will try to GET from the configured JOB_ITEM_SHARE_DATA_GET
// endpoint (environment variable). Returns nil, error when not found.
func ShareDataGet(taskID, key string) ([]string, error) {
	now := time.Now()
	storeMu.Lock()
	var res []string
	if _, ok := store[taskID]; ok {
		entries := store[taskID][key]
		var kept []shareEntry
		for _, e := range entries {
			if !e.ExpiresAt.IsZero() && now.After(e.ExpiresAt) {
				// expired: skip
				continue
			}
			res = append(res, e.Data)
			kept = append(kept, e)
		}
		// replace with kept (prune expired)
		store[taskID][key] = kept
	}
	storeMu.Unlock()

	if len(res) > 0 {
		return res, nil
	}

	// Fallback to remote GET endpoint
	getURL := os.Getenv("JOB_ITEM_SHARE_DATA_GET")
	if getURL == "" {
		return nil, fmt.Errorf("not found")
	}
	// replace placeholder or append key
	if strings.Contains(getURL, ":key") {
		getURL = strings.ReplaceAll(getURL, ":key", key)
	} else {
		if strings.HasSuffix(getURL, "/") {
			getURL = getURL + key
		} else {
			getURL = getURL + "/" + key
		}
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(getURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote returned status %d", resp.StatusCode)
	}

	// decode possible shapes: {"data": [...]}, {"data":"string"}, or top-level array
	var body interface{}
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&body); err != nil {
		return nil, err
	}

	// try to extract data
	switch v := body.(type) {
	case map[string]interface{}:
		if d, ok := v["data"]; ok {
			switch d2 := d.(type) {
			case []interface{}:
				out := make([]string, 0, len(d2))
				for _, it := range d2 {
					out = append(out, fmt.Sprint(it))
				}
				return out, nil
			default:
				return []string{fmt.Sprint(d2)}, nil
			}
		}
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, it := range v {
			out = append(out, fmt.Sprint(it))
		}
		return out, nil
	}

	return nil, fmt.Errorf("not found")
}

// janitor periodically prunes expired entries
func init() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			pruneExpired()
		}
	}()
}

func pruneExpired() {
	now := time.Now()
	storeMu.Lock()
	defer storeMu.Unlock()
	for taskID, keys := range store {
		for key, entries := range keys {
			var kept []shareEntry
			for _, e := range entries {
				if !e.ExpiresAt.IsZero() && now.After(e.ExpiresAt) {
					continue
				}
				kept = append(kept, e)
			}
			if len(kept) == 0 {
				delete(store[taskID], key)
			} else {
				store[taskID][key] = kept
			}
		}
		if len(store[taskID]) == 0 {
			delete(store, taskID)
		}
	}
}
