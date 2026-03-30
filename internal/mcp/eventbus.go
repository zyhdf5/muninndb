package mcp

import (
	"encoding/json"
	"sync/atomic"
)

// MCPEvent is a cognitive notification sent from the engine to connected AI tools.
// Params must be pre-serialized JSON — marshaling errors are caught at the emission site.
type MCPEvent struct {
	Vault  string          `json:"vault"`
	Method string          `json:"method"` // e.g. "notifications/muninn/contradiction"
	Params json.RawMessage `json:"params"`
}

// EventBus delivers cognitive events from the engine to the MCP server.
// There is exactly one subscriber: MCPServer. Events() returns the same channel on every call.
type EventBus interface {
	Publish(event MCPEvent)
	Events() <-chan MCPEvent
	BusDropped() int64 // events dropped at bus level (buffer full)
	Close()
}

// NoopEventBus is a nil-safe EventBus used when push is disabled.
type NoopEventBus struct{}

func (NoopEventBus) Publish(MCPEvent)        {}
func (NoopEventBus) Events() <-chan MCPEvent { return nil }
func (NoopEventBus) BusDropped() int64       { return 0 }
func (NoopEventBus) Close()                  {}

type chanEventBus struct {
	ch      chan MCPEvent
	dropped atomic.Int64
}

// NewEventBus creates an EventBus with the given channel buffer size.
// bufSize of 256 is recommended for production.
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
