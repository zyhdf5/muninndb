package replication

import "sync"

type FrameHandler func(fromNodeID string, payload []byte) error

type ConnManager struct {
	localNodeID string
	peers       map[string]*PeerConn
	mu          sync.RWMutex

	handlersMu sync.RWMutex
	handlers   map[uint8]FrameHandler
}

func NewConnManager(localNodeID string) *ConnManager {
	return &ConnManager{
		localNodeID: localNodeID,
		peers:       make(map[string]*PeerConn),
		handlers:    make(map[uint8]FrameHandler),
	}
}

func (m *ConnManager) RegisterHandler(frameType uint8, h FrameHandler) {
	m.handlersMu.Lock()
	defer m.handlersMu.Unlock()
	m.handlers[frameType] = h
}

func (m *ConnManager) Dispatch(fromNodeID string, frameType uint8, payload []byte) error {
	m.handlersMu.RLock()
	h := m.handlers[frameType]
	m.handlersMu.RUnlock()
	if h == nil {
		return nil
	}
	return h(fromNodeID, payload)
}

func (m *ConnManager) AddPeer(nodeID, addr string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.peers[nodeID]; ok {
		_ = existing.Close()
	}
	m.peers[nodeID] = NewPeerConn(nodeID, addr)
}

func (m *ConnManager) RemovePeer(nodeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.peers[nodeID]; ok {
		_ = p.Close()
		delete(m.peers, nodeID)
	}
}

func (m *ConnManager) GetPeer(nodeID string) (*PeerConn, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.peers[nodeID]
	return p, ok
}

func (m *ConnManager) Peers() []*PeerConn {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*PeerConn, 0, len(m.peers))
	for _, p := range m.peers {
		out = append(out, p)
	}
	return out
}

func (m *ConnManager) Broadcast(frameType uint8, payload []byte) map[string]error {
	m.mu.RLock()
	snapshot := make([]*PeerConn, 0, len(m.peers))
	for _, p := range m.peers {
		snapshot = append(snapshot, p)
	}
	m.mu.RUnlock()

	errs := make(map[string]error)
	for _, p := range snapshot {
		if !p.IsConnected() {
			continue
		}
		if err := p.Send(frameType, payload); err != nil {
			errs[p.NodeID()] = err
		}
	}
	return errs
}

func (m *ConnManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, p := range m.peers {
		_ = p.Close()
		delete(m.peers, id)
	}
	return nil
}
