package mcp

import (
	"testing"
	"time"
)

func TestNoopEventBus(t *testing.T) {
	// given: Noop 总线
	b := NoopEventBus{}

	// when: 发布事件
	b.Publish(MCPEvent{Vault: "v", Method: "m"})

	// then: 不阻塞且无事件通道
	if b.Events() != nil || b.BusDropped() != 0 {
		t.Fatalf("noop bus unexpected state")
	}
}

func TestChanEventBus_RoundTripAndClose(t *testing.T) {
	// given: 带缓冲事件总线
	b := NewEventBus(1)
	defer b.Close()

	// when: 发布并消费事件
	e := MCPEvent{Vault: "v1", Method: "remember"}
	b.Publish(e)
	select {
	case got := <-b.Events():
		// then: 事件应完整往返
		if got.Method != e.Method || got.Vault != e.Vault {
			t.Fatalf("event mismatch: %+v vs %+v", got, e)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting event")
	}
}

func TestChanEventBus_DroppedCounter(t *testing.T) {
	// given: 容量为 1 的总线
	b := NewEventBus(1)
	defer b.Close()

	// when: 连续发布 3 次，至少 2 次会丢弃
	b.Publish(MCPEvent{Method: "1"})
	b.Publish(MCPEvent{Method: "2"})
	b.Publish(MCPEvent{Method: "3"})

	// then: dropped 计数应增长
	if b.BusDropped() < 1 {
		t.Fatalf("expected dropped > 0, got %d", b.BusDropped())
	}
}
