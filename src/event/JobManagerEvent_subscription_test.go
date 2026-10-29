package event

// Regression test WO-2026-10-29-001 Task 6: RefreshPubSub must release previous
// subscriptions before re-subscribing so one restart event cannot be amplified into
// N restart cycles.

import (
	"sync"
	"testing"

	"job_item/support"
)

type countingBroker struct {
	mu         sync.Mutex
	key        string
	active     int
	subCalls   int
	basicCalls int
	unsubCalls int
}

func (b *countingBroker) track() func() {
	b.mu.Lock()
	b.active++
	b.mu.Unlock()
	return func() {
		b.mu.Lock()
		b.active--
		b.unsubCalls++
		b.mu.Unlock()
	}
}

func (b *countingBroker) Pub(string, string) {}
func (b *countingBroker) Sub(_, _ string, _ func(string)) (func(), error) {
	b.mu.Lock()
	b.subCalls++
	b.mu.Unlock()
	return b.track(), nil
}
func (b *countingBroker) SubSync(string, string, func(string, error), support.SubSyncOpts) (bool, error) {
	return false, nil
}
func (b *countingBroker) GetBroker_P() any { return b }
func (b *countingBroker) SetKey_P(key string) {
	b.mu.Lock()
	b.key = key
	b.mu.Unlock()
}
func (b *countingBroker) GetKey_P() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.key
}
func (b *countingBroker) IsConnected() bool        { return true }
func (b *countingBroker) GetRefreshPubSub() string { return "refresh" }
func (b *countingBroker) BasicSub(string, func(string)) (func(), error) {
	b.mu.Lock()
	b.basicCalls++
	b.mu.Unlock()
	return b.track(), nil
}
func (b *countingBroker) BasicSubSync(string, func(string, error), support.SubSyncOpts) (bool, error) {
	return false, nil
}

func (b *countingBroker) snapshot() (active, subCalls, basicCalls, unsubCalls int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.active, b.subCalls, b.basicCalls, b.unsubCalls
}

func TestListenEventRefreshKeepsSubscriptionCountStable(t *testing.T) {
	support.Helper = support.SupportConstruct("Test")
	support.Helper.EventBus = support.EventBusConstruct()
	support.Helper.ConfigYaml = &support.ConfigYamlSupport{ConfigData: support.ConfigData{
		Uuid:        "project-1",
		Identity_id: "identity-1",
		Jobs:        []support.ConfigJob{{Name: "job", Event: "evt", Cmd: "echo"}},
	}}

	t.Setenv("JOB_ITEM_DATA_DIR", t.TempDir())
	stateStoreOnce = sync.Once{}
	stateStoreInst = nil

	broker := &countingBroker{}
	brokerSupport := support.BrokerConnectionSupportContruct()
	brokerSupport.RegisterConnection("key-1", broker)
	support.Helper.BrokerConnection = brokerSupport

	jm := JobManagerEventConstruct()
	jm.ListenEvent("key-1")

	active, _, _, _ := broker.snapshot()
	if active == 0 {
		t.Fatal("expected active subscriptions after ListenEvent")
	}
	initialActive := active

	for i := 0; i < 5; i++ {
		support.Helper.EventBus.GetBus().Publish("refresh", nil)
	}

	activeAfter, subCalls, basicCalls, unsubCalls := broker.snapshot()
	if activeAfter != initialActive {
		t.Fatalf("active subscriptions not stable: initial=%d after=%d (sub=%d basic=%d unsub=%d)",
			initialActive, activeAfter, subCalls, basicCalls, unsubCalls)
	}
}
