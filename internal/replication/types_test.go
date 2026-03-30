package replication

import (
	"testing"
	"time"
)

func TestConsistencyMode_Values(t *testing.T) {
	if ModeEventual != 1 {
		t.Errorf("ModeEventual = %d, want 1", ModeEventual)
	}
	if ModeStrong != 2 {
		t.Errorf("ModeStrong = %d, want 2", ModeStrong)
	}
	if ModeBoundedStaleness != 3 {
		t.Errorf("ModeBoundedStaleness = %d, want 3", ModeBoundedStaleness)
	}
}

func TestNodeInfo_Fields(t *testing.T) {
	exp := time.Now().Add(10 * time.Second)
	n := NodeInfo{
		NodeID:   "cortex-01",
		Addr:     "10.0.0.1:7946",
		Role:     RolePrimary,
		LastSeq:  42,
		LeaseExp: exp,
	}
	if n.NodeID != "cortex-01" {
		t.Errorf("NodeID = %q, want cortex-01", n.NodeID)
	}
	if n.Role != RolePrimary {
		t.Errorf("Role = %v, want RolePrimary", n.Role)
	}
	if n.LastSeq != 42 {
		t.Errorf("LastSeq = %d, want 42", n.LastSeq)
	}
	if !n.LeaseExp.Equal(exp) {
		t.Errorf("LeaseExp mismatch")
	}
}

func TestReplicationEntry_Fields(t *testing.T) {
	e := ReplicationEntry{
		Seq:         100,
		Op:          OpSet,
		Key:         []byte("hello"),
		Value:       []byte("world"),
		TimestampNS: 1234567890,
	}
	if e.Seq != 100 {
		t.Errorf("Seq = %d, want 100", e.Seq)
	}
	if e.Op != OpSet {
		t.Errorf("Op = %v, want OpSet", e.Op)
	}
	if string(e.Key) != "hello" {
		t.Errorf("Key = %q, want hello", e.Key)
	}
	if string(e.Value) != "world" {
		t.Errorf("Value = %q, want world", e.Value)
	}
}
