package replication

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
)

var ErrNotConnected = errors.New("peer not connected")

type PeerConn struct {
	nodeID string
	addr   string
	conn   net.Conn
	mu     sync.Mutex
	closed bool
}

func NewPeerConn(nodeID, addr string) *PeerConn {
	return &PeerConn{nodeID: nodeID, addr: addr}
}

func (p *PeerConn) NodeID() string { return p.nodeID }
func (p *PeerConn) Addr() string   { return p.addr }

func (p *PeerConn) IsConnected() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.conn != nil && !p.closed
}

func (p *PeerConn) Connect(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return errors.New("peer conn is closed")
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", p.addr)
	if err != nil {
		return fmt.Errorf("dial %s: %w", p.addr, err)
	}
	if p.conn != nil {
		_ = p.conn.Close()
	}
	p.conn = conn
	return nil
}

func (p *PeerConn) Send(frameType uint8, payload []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn == nil || p.closed {
		return ErrNotConnected
	}
	f := &Frame{Version: 0x01, Type: frameType, PayloadLength: uint32(len(payload)), Payload: payload}
	return WriteFrame(p.conn, f)
}

func (p *PeerConn) Receive() (frameType uint8, payload []byte, err error) {
	p.mu.Lock()
	conn := p.conn
	closed := p.closed
	p.mu.Unlock()
	if conn == nil || closed {
		return 0, nil, ErrNotConnected
	}
	f, err := ReadFrame(conn)
	if err != nil {
		p.mu.Lock()
		isClosed := p.closed
		p.mu.Unlock()
		if isClosed {
			return 0, nil, ErrNotConnected
		}
		return 0, nil, err
	}
	return f.Type, f.Payload, nil
}

func (p *PeerConn) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	if p.conn != nil {
		err := p.conn.Close()
		p.conn = nil
		return err
	}
	return nil
}
