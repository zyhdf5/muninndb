package mcp

import (
	"encoding/json"
	"sync/atomic"
)

// MCPEvent 表示 MCP 层事件。
type MCPEvent struct {
	Vault  string          `json:"vault"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

// EventBus 定义事件总线能力。
type EventBus interface {
	Publish(event MCPEvent)
	Events() <-chan MCPEvent
	BusDropped() int64
	Close()
}

// NoopEventBus 是空实现。
type NoopEventBus struct{}

func (NoopEventBus) Publish(MCPEvent)        {}
func (NoopEventBus) Events() <-chan MCPEvent { return nil }
func (NoopEventBus) BusDropped() int64       { return 0 }
func (NoopEventBus) Close()                  {}

type chanEventBus struct {
	ch      chan MCPEvent
	dropped atomic.Int64
}

// NewEventBus 创建带缓冲的事件总线。
func NewEventBus(bufSize int) EventBus {
	return &chanEventBus{ch: make(chan MCPEvent, bufSize)}
}

func (b *chanEventBus) Publish(event MCPEvent) {
	select {
	case b.ch <- event:
	default:
		b.dropped.Add(1)
	}
}

func (b *chanEventBus) Events() <-chan MCPEvent { return b.ch }
func (b *chanEventBus) BusDropped() int64       { return b.dropped.Load() }
func (b *chanEventBus) Close()                  { close(b.ch) }
