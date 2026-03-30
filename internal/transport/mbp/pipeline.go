package mbp

import (
	"sync"
)

// PendingMap tracks in-flight requests by correlation ID.
// Thread-safe. Used by the connection handler to route responses
// back to the correct waiting goroutine.
type PendingMap struct {
	mu   sync.RWMutex
	reqs map[uint64]chan *Frame
}

// NewPendingMap creates a new correlation ID router.
func NewPendingMap() *PendingMap {
	return &PendingMap{
		reqs: make(map[uint64]chan *Frame),
	}
}

// Register registers a pending request and returns a channel that will
// receive the response frame. The channel is buffered (size 1).
func (p *PendingMap) Register(correlationID uint64) <-chan *Frame {
	p.mu.Lock()
	defer p.mu.Unlock()

	ch := make(chan *Frame, 1)
	p.reqs[correlationID] = ch
	return ch
}

// Complete delivers a response frame to the waiting goroutine.
// If the correlation ID is not registered, the frame is dropped.
func (p *PendingMap) Complete(correlationID uint64, resp *Frame) {
	p.mu.Lock()
	ch, exists := p.reqs[correlationID]
	p.mu.Unlock()

	if exists {
		select {
		case ch <- resp:
		default:
			// Channel is full or closed; drop the frame
		}
	}
}

// Cancel removes the pending request, closing the channel.
// Any goroutine waiting on the channel will unblock.
func (p *PendingMap) Cancel(correlationID uint64) {
	p.mu.Lock()
	ch, exists := p.reqs[correlationID]
	if exists {
		delete(p.reqs, correlationID)
	}
	p.mu.Unlock()

	if exists {
		close(ch)
	}
}
