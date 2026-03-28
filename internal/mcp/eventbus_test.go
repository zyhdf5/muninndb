package mcp

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEventBus_PublishAndReceive(t *testing.T) {
	bus := NewEventBus(8)
	defer bus.Close()

	params, _ := json.Marshal(map[string]string{"a": "1"})
	evt := MCPEvent{Vault: "test", Method: "notifications/muninn/test", Params: params}
	bus.Publish(evt)

	select {
	case got := <-bus.Events():
		if got.Vault != "test" || got.Method != "notifications/muninn/test" {
			t.Fatalf("unexpected event: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestEventBus_DropsWhenFull(t *testing.T) {
	bus := NewEventBus(1)
	defer bus.Close()

	params, _ := json.Marshal(map[string]string{})
	evt := MCPEvent{Vault: "v", Method: "m", Params: params}

	bus.Publish(evt) // fills buffer
	bus.Publish(evt) // should drop

	if bus.BusDropped() != 1 {
		t.Fatalf("expected 1 dropped, got %d", bus.BusDropped())
	}
}

func TestEventBus_CloseTerminatesEvents(t *testing.T) {
	bus := NewEventBus(8)
	bus.Close()
	// Events() channel should be closed (range terminates)
	count := 0
	for range bus.Events() {
		count++
	}
	if count != 0 {
		t.Fatalf("expected 0 events after close, got %d", count)
	}
}

func TestNoopEventBus(t *testing.T) {
	var bus EventBus = NoopEventBus{}
	// Should not panic
	bus.Publish(MCPEvent{Vault: "v", Method: "m"})
	if bus.BusDropped() != 0 {
		t.Fatal("noop bus should always return 0 dropped")
	}
	if bus.Events() != nil {
		t.Fatal("noop bus Events() should return nil")
	}
	bus.Close() // should not panic
}
