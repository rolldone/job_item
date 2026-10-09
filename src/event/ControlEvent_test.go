package event

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"job_item/support"
)

type controlTestBroker struct {
	mu        sync.Mutex
	published map[string][]string
	subTopic  string
	subFn     func(string)
}

func newControlTestBroker() *controlTestBroker {
	return &controlTestBroker{published: make(map[string][]string)}
}

func (b *controlTestBroker) Pub(topic, message string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.published[topic] = append(b.published[topic], message)
}
func (b *controlTestBroker) Sub(string, string, func(string)) (func(), error) {
	return func() {}, nil
}
func (b *controlTestBroker) SubSync(string, string, func(string, error), support.SubSyncOpts) (bool, error) {
	return false, nil
}
func (b *controlTestBroker) GetBroker_P() any         { return b }
func (b *controlTestBroker) SetKey_P(string)          {}
func (b *controlTestBroker) GetKey_P() string         { return "control-test" }
func (b *controlTestBroker) IsConnected() bool        { return true }
func (b *controlTestBroker) GetRefreshPubSub() string { return "refresh" }
func (b *controlTestBroker) BasicSub(topic string, callback func(string)) (func(), error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subTopic = topic
	b.subFn = callback
	return func() {
		b.mu.Lock()
		b.subFn = nil
		b.mu.Unlock()
	}, nil
}
func (b *controlTestBroker) BasicSubSync(string, func(string, error), support.SubSyncOpts) (bool, error) {
	return false, nil
}

func (b *controlTestBroker) response(topic string) []ControlResponse {
	b.mu.Lock()
	defer b.mu.Unlock()
	messages := b.published[topic]
	out := make([]ControlResponse, 0, len(messages))
	for _, message := range messages {
		var response ControlResponse
		if err := json.Unmarshal([]byte(message), &response); err != nil {
			continue
		}
		out = append(out, response)
	}
	return out
}

func TestHandleControlMessage_ReturnsPersistedProcessStatuses(t *testing.T) {
	st := setupEventTest(t)
	if err := st.SetRunning("check-running", "event.running"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetRunning("check-finished", "event.finished"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetFinal("check-finished", "event.finished", support.ProcessStatusFinished, 0, ""); err != nil {
		t.Fatal(err)
	}
	if err := st.SetRunning("check-failed", "event.failed"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetFinal("check-failed", "event.failed", support.ProcessStatusFailed, 3, "stderr"); err != nil {
		t.Fatal(err)
	}

	support.Helper.ConfigYaml.ConfigData.Identity_id = "worker-identity-1"
	broker := newControlTestBroker()
	cases := []struct {
		name  string
		jobID string
		event string
		want  string
	}{
		{name: "running", jobID: "check-running", event: "event.running", want: support.ProcessStatusRunning},
		{name: "finished", jobID: "check-finished", event: "event.finished", want: support.ProcessStatusFinished},
		{name: "failed", jobID: "check-failed", event: "event.failed", want: support.ProcessStatusFailed},
		{name: "not found", jobID: "absent-job", event: "event.absent", want: ControlStatusNotFound},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requestID := []string{"req-running", "req-finished", "req-failed", "req-absent"}[i]
			payload, err := json.Marshal(ControlRequest{
				Action: ControlActionCheckStatus, RequestID: requestID, JobID: tc.jobID, Event: tc.event,
			})
			if err != nil {
				t.Fatal(err)
			}
			got := handleControlMessage(broker, string(payload))
			if got != tc.want {
				t.Fatalf("status = %q, want %q", got, tc.want)
			}
			responses := broker.response(requestID + ".control_response")
			if len(responses) != 1 {
				t.Fatalf("response count = %d, want 1", len(responses))
			}
			response := responses[0]
			if response.RequestID != requestID || response.JobID != tc.jobID || response.IdentityID != "worker-identity-1" || response.Status != tc.want || response.Event != tc.event {
				t.Fatalf("response contract mismatch: %+v", response)
			}
			if _, err := time.Parse(time.RFC3339, response.CheckedAt); err != nil {
				t.Fatalf("checked_at is not RFC3339: %q (%v)", response.CheckedAt, err)
			}
		})
	}
}

func TestHandleControlMessage_IgnoresMalformedAndUnknownRequests(t *testing.T) {
	setupEventTest(t)
	broker := newControlTestBroker()
	invalid := []string{
		"{not-json",
		`{"action":"unsupported","request_id":"req-1","job_id":"job-1"}`,
		`{"action":"check_status","job_id":"job-1"}`,
	}
	for _, payload := range invalid {
		if got := handleControlMessage(broker, payload); got != "" {
			t.Errorf("invalid request returned status %q", got)
		}
	}
	if len(broker.published) != 0 {
		t.Fatalf("invalid requests must not publish response, got %v", broker.published)
	}
}

func TestListenControlTopic_SubscriptionIsWorkerScopedAndUnsubscribes(t *testing.T) {
	setupEventTest(t)
	support.Helper.ConfigYaml.ConfigData.Identity_id = "7bd0c868-87a0-4c90-8d27-559677763bb6"
	broker := newControlTestBroker()
	event := &JobManagerEvent{}

	unsub := event.listenControlTopic(broker)
	if broker.subTopic != support.Helper.ConfigYaml.ConfigData.Identity_id+".control" {
		t.Fatalf("subscribed topic = %q", broker.subTopic)
	}
	if broker.subFn == nil {
		t.Fatal("BasicSub callback not registered")
	}
	callback := broker.subFn
	callback(`{"action":"check_status","request_id":"req-worker","job_id":"missing","event":"task.run"}`)
	responses := broker.response("req-worker.control_response")
	if len(responses) != 1 || responses[0].Status != ControlStatusNotFound {
		t.Fatalf("listener callback response = %+v", responses)
	}
	unsub()
	if broker.subFn != nil {
		t.Fatal("unsubscribe did not remove test subscription")
	}
}

func TestHandleControlMessage_ResponseTopicsAreIsolated(t *testing.T) {
	setupEventTest(t)
	broker := newControlTestBroker()
	for _, requestID := range []string{"req-A", "req-B"} {
		payload, _ := json.Marshal(ControlRequest{
			Action: ControlActionCheckStatus, RequestID: requestID, JobID: "missing-" + requestID,
		})
		handleControlMessage(broker, string(payload))
	}
	for _, requestID := range []string{"req-A", "req-B"} {
		responses := broker.response(requestID + ".control_response")
		if len(responses) != 1 || responses[0].RequestID != requestID {
			t.Fatalf("response topic for %s is not isolated: %+v", requestID, responses)
		}
	}
}

func TestLookupJobStatus_UsesMemoryFallbackWhenStoreUnavailable(t *testing.T) {
	setupEventTest(t)
	previousStore := support.ProcessStateStoreGlobal()
	support.RegisterProcessStateStore(nil)
	t.Cleanup(func() { support.RegisterProcessStateStore(previousStore) })
	registrySet("memory-only-job", support.ProcessStatusFailed)

	status, eventName := lookupJobStatus(ControlRequest{
		JobID: "memory-only-job",
		Event: "event.from.request",
	})
	if status != support.ProcessStatusFailed {
		t.Fatalf("fallback status = %q, want FAILED", status)
	}
	if eventName != "event.from.request" {
		t.Fatalf("fallback event = %q, want request event", eventName)
	}
}
