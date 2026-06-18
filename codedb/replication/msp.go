package replication

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vmihailenco/msgpack/v5"
)

type PeerState struct {
	NodeID      string
	Addr        string
	Role        NodeRole
	LastSeen    time.Time
	MissedBeats int
	SDown       bool
}

type MSP struct {
	localNodeID     string
	localAddr       string
	mgr             *ConnManager
	peers           map[string]*PeerState
	mu              sync.RWMutex
	interval        time.Duration
	missedThreshold atomic.Int32
	votedDown       map[string]map[string]struct{}
	OnSDown         func(nodeID string)
	OnODown         func(nodeID string)
	OnRecover       func(nodeID string)
	OnAddrChanged   func(nodeID, newAddr string)
}

type pingPayload struct {
	NodeID string `msgpack:"node_id"`
	Addr   string `msgpack:"addr,omitempty"`
}

func NewMSP(localNodeID, localAddr string, mgr *ConnManager) *MSP {
	return &MSP{localNodeID: localNodeID, localAddr: localAddr, mgr: mgr, peers: map[string]*PeerState{}, votedDown: map[string]map[string]struct{}{}}
}

func (m *MSP) AddPeer(nodeID, addr string, role NodeRole) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.peers[nodeID] = &PeerState{NodeID: nodeID, Addr: addr, Role: role, LastSeen: time.Now()}
}
func (m *MSP) RemovePeer(nodeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.peers, nodeID)
	delete(m.votedDown, nodeID)
}
func (m *MSP) HandlePing(fromNodeID string, payload []byte) {
	m.handleHeartbeat(fromNodeID, m.decodeAddr(payload))
}
func (m *MSP) HandlePong(fromNodeID string, payload []byte) {
	m.handleHeartbeat(fromNodeID, m.decodeAddr(payload))
}

func (m *MSP) decodeAddr(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	var p pingPayload
	if err := msgpack.Unmarshal(payload, &p); err != nil {
		return ""
	}
	return p.Addr
}

func (m *MSP) handleHeartbeat(fromNodeID, fromAddr string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.peers[fromNodeID]
	if !ok {
		return
	}
	wasSDown := p.SDown
	p.LastSeen = time.Now()
	p.MissedBeats = 0
	p.SDown = false
	if fromAddr != "" && fromAddr != p.Addr {
		p.Addr = fromAddr
		if m.OnAddrChanged != nil {
			go m.OnAddrChanged(fromNodeID, fromAddr)
		}
	}
	if wasSDown && m.OnRecover != nil {
		go m.OnRecover(fromNodeID)
	}
}

func (m *MSP) IsSDown(nodeID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.peers[nodeID]
	return ok && p.SDown
}
func (m *MSP) IsODown(nodeID string, quorum int) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.peers[nodeID]
	if !ok || !p.SDown {
		return false
	}
	votes := 1
	if v, ok := m.votedDown[nodeID]; ok {
		votes += len(v)
	}
	return votes >= quorum
}
func (m *MSP) LivePeers() []*PeerState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []*PeerState{}
	for _, p := range m.peers {
		if !p.SDown {
			cp := *p
			out = append(out, &cp)
		}
	}
	return out
}
func (m *MSP) AllPeers() []*PeerState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []*PeerState{}
	for _, p := range m.peers {
		cp := *p
		out = append(out, &cp)
	}
	return out
}

func (m *MSP) Run(ctx context.Context, pingInterval time.Duration, missedThreshold int) error {
	m.missedThreshold.Store(int32(missedThreshold))
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	payload, err := msgpack.Marshal(pingPayload{NodeID: m.localNodeID, Addr: m.localAddr})
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			m.tick(payload, pingInterval, int(m.missedThreshold.Load()))
		}
	}
}

func (m *MSP) SetMissedThreshold(n int) { m.missedThreshold.Store(int32(n)) }
func (m *MSP) nonObserverQuorumLocked() int {
	count := 1
	for _, p := range m.peers {
		if p.Role != RoleObserver {
			count++
		}
	}
	return count/2 + 1
}
func (m *MSP) NonObserverQuorum() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.nonObserverQuorumLocked()
}
func (m *MSP) SetHeartbeatInterval(d time.Duration) { m.mu.Lock(); defer m.mu.Unlock(); m.interval = d }

func (m *MSP) tick(pingPayload []byte, pingInterval time.Duration, missedThreshold int) {
	m.mgr.Broadcast(TypePing, pingPayload)
	m.mu.Lock()
	defer m.mu.Unlock()
	effectiveQuorum := m.nonObserverQuorumLocked()
	sdownThreshold := time.Duration(missedThreshold) * pingInterval
	for _, p := range m.peers {
		p.MissedBeats = int(time.Since(p.LastSeen) / pingInterval)
		if time.Since(p.LastSeen) > sdownThreshold && !p.SDown {
			p.SDown = true
			nodeID := p.NodeID
			if m.OnSDown != nil {
				go m.OnSDown(nodeID)
			}
			if p.Role != RoleObserver {
				votes := 1
				if voters, ok := m.votedDown[nodeID]; ok {
					votes += len(voters)
				}
				if votes >= effectiveQuorum && m.OnODown != nil {
					go m.OnODown(nodeID)
				}
			}
		}
	}
}
