package replication

import (
	"sync"
	"testing"
	"time"

	"github.com/vmihailenco/msgpack/v5"
)

func mustMarshalPingPayload(t *testing.T, p pingPayload) []byte {
	t.Helper()
	b, err := msgpack.Marshal(p)
	if err != nil {
		t.Fatalf("mustMarshalPingPayload: %v", err)
	}
	return b
}

// newTestMSP creates an MSP with a ConnManager but no real connections.
func newTestMSP(localID string) *MSP {
	mgr := NewConnManager(localID)
	return NewMSP(localID, "127.0.0.1:9000", mgr)
}

func TestMSP_AddRemovePeer(t *testing.T) {
	m := newTestMSP("node-A")

	m.AddPeer("node-B", "127.0.0.1:7001", RoleReplica)
	m.AddPeer("node-C", "127.0.0.1:7002", RoleSentinel)

	all := m.AllPeers()
	if len(all) != 2 {
		t.Fatalf("expected 2 peers, got %d", len(all))
	}

	m.RemovePeer("node-B")
	all = m.AllPeers()
	if len(all) != 1 {
		t.Fatalf("expected 1 peer after remove, got %d", len(all))
	}
	if all[0].NodeID != "node-C" {
		t.Errorf("expected node-C to remain, got %s", all[0].NodeID)
	}
}

func TestMSP_HandlePing_ClearsMissedBeats(t *testing.T) {
	m := newTestMSP("node-A")
	m.AddPeer("node-B", "127.0.0.1:7001", RoleReplica)

	// Manually set MissedBeats to simulate prior missed heartbeats.
	m.mu.Lock()
	m.peers["node-B"].MissedBeats = 2
	m.mu.Unlock()

	m.HandlePing("node-B", nil)

	m.mu.RLock()
	beats := m.peers["node-B"].MissedBeats
	m.mu.RUnlock()

	if beats != 0 {
		t.Errorf("expected MissedBeats=0 after HandlePing, got %d", beats)
	}
}

func TestMSP_SDown_After3MissedBeats(t *testing.T) {
	m := newTestMSP("node-A")
	m.AddPeer("node-B", "127.0.0.1:7001", RoleReplica)

	var mu sync.Mutex
	sdownFired := false
	m.OnSDown = func(nodeID string) {
		mu.Lock()
		defer mu.Unlock()
		if nodeID == "node-B" {
			sdownFired = true
		}
	}

	// Manually drive MissedBeats to the threshold.
	m.mu.Lock()
	p := m.peers["node-B"]
	p.MissedBeats = 2
	m.mu.Unlock()

	// One more miss pushes it to 3 and triggers SDOWN.
	m.mu.Lock()
	p.MissedBeats++
	if p.MissedBeats >= 3 && !p.SDown {
		p.SDown = true
		if m.OnSDown != nil {
			go m.OnSDown(p.NodeID)
		}
	}
	m.mu.Unlock()

	// Give the goroutine time to fire.
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	fired := sdownFired
	mu.Unlock()

	if !fired {
		t.Error("expected OnSDown to be called")
	}

	if !m.IsSDown("node-B") {
		t.Error("expected node-B to be SDOWN")
	}
}

func TestMSP_Recover_ClearsSDown(t *testing.T) {
	m := newTestMSP("node-A")
	m.AddPeer("node-B", "127.0.0.1:7001", RoleReplica)

	// Put the peer into SDOWN.
	m.mu.Lock()
	m.peers["node-B"].SDown = true
	m.peers["node-B"].MissedBeats = 3
	m.mu.Unlock()

	var mu sync.Mutex
	recoverFired := false
	m.OnRecover = func(nodeID string) {
		mu.Lock()
		defer mu.Unlock()
		if nodeID == "node-B" {
			recoverFired = true
		}
	}

	m.HandlePong("node-B", nil)

	// Give the goroutine time to fire.
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	fired := recoverFired
	mu.Unlock()

	if !fired {
		t.Error("expected OnRecover to be called")
	}

	if m.IsSDown("node-B") {
		t.Error("expected node-B to no longer be SDOWN after HandlePong")
	}
}

func TestMSP_LivePeers_ExcludesSDown(t *testing.T) {
	m := newTestMSP("node-A")
	m.AddPeer("node-B", "127.0.0.1:7001", RoleReplica)
	m.AddPeer("node-C", "127.0.0.1:7002", RoleReplica)
	m.AddPeer("node-D", "127.0.0.1:7003", RoleSentinel)

	// Mark node-C as SDOWN.
	m.mu.Lock()
	m.peers["node-C"].SDown = true
	m.mu.Unlock()

	live := m.LivePeers()
	if len(live) != 2 {
		t.Errorf("expected 2 live peers, got %d", len(live))
	}
	for _, p := range live {
		if p.NodeID == "node-C" {
			t.Error("node-C (SDOWN) should not appear in LivePeers")
		}
	}
}

func TestMSP_IsODown_QuorumCheck(t *testing.T) {
	m := newTestMSP("node-A")
	m.AddPeer("node-B", "127.0.0.1:7001", RoleReplica)

	// node-B is not SDOWN yet — IsODown must be false.
	if m.IsODown("node-B", 2) {
		t.Error("expected IsODown=false when peer is not SDOWN")
	}

	// Mark node-B as SDOWN locally.
	m.mu.Lock()
	m.peers["node-B"].SDown = true
	m.mu.Unlock()

	// With quorum=1 (only this node needed), IsODown should be true.
	if !m.IsODown("node-B", 1) {
		t.Error("expected IsODown=true with quorum=1 and local SDOWN")
	}

	// With quorum=2 and no gossip votes, local view alone is not enough.
	if m.IsODown("node-B", 2) {
		t.Error("expected IsODown=false with quorum=2 and only local vote")
	}

	// Simulate a gossip vote from node-C.
	m.mu.Lock()
	m.votedDown["node-B"] = map[string]struct{}{"node-C": {}}
	m.mu.Unlock()

	if !m.IsODown("node-B", 2) {
		t.Error("expected IsODown=true with quorum=2 and 2 votes (local + node-C)")
	}
}

func TestMSP_AddrUpdate_OnHeartbeat(t *testing.T) {
	m := newTestMSP("node-A")
	m.AddPeer("node-B", "10.0.1.1:9000", RoleReplica)

	var gotNode, gotAddr string
	var cbMu sync.Mutex
	m.OnAddrChanged = func(nodeID, newAddr string) {
		cbMu.Lock()
		defer cbMu.Unlock()
		gotNode = nodeID
		gotAddr = newAddr
	}

	// Simulate a PING from node-B advertising a new address.
	// The pingPayload is accessible within package replication.
	payload := mustMarshalPingPayload(t, pingPayload{NodeID: "node-B", Addr: "10.0.2.5:9000"})
	m.HandlePing("node-B", payload)

	// Wait for the async OnAddrChanged callback.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		cbMu.Lock()
		done := gotNode != ""
		cbMu.Unlock()
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	cbMu.Lock()
	defer cbMu.Unlock()
	if gotNode != "node-B" || gotAddr != "10.0.2.5:9000" {
		t.Errorf("OnAddrChanged: got node=%q addr=%q, want node-B / 10.0.2.5:9000", gotNode, gotAddr)
	}
	for _, p := range m.AllPeers() {
		if p.NodeID == "node-B" && p.Addr != "10.0.2.5:9000" {
			t.Errorf("PeerState.Addr = %q, want 10.0.2.5:9000", p.Addr)
		}
	}
}

func TestMSP_AddrUpdate_LegacyPing_NoChange(t *testing.T) {
	m := newTestMSP("node-A")
	m.AddPeer("node-B", "10.0.1.1:9000", RoleReplica)

	addrChangedFired := false
	m.OnAddrChanged = func(_, _ string) { addrChangedFired = true }

	// Legacy PING: nil payload — addr should remain unchanged.
	m.HandlePing("node-B", nil)
	time.Sleep(50 * time.Millisecond)

	if addrChangedFired {
		t.Error("OnAddrChanged should NOT fire for a legacy (nil payload) PING")
	}
	for _, p := range m.AllPeers() {
		if p.NodeID == "node-B" && p.Addr != "10.0.1.1:9000" {
			t.Errorf("addr changed unexpectedly: %q", p.Addr)
		}
	}
}
