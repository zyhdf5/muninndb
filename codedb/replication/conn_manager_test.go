package replication

import "testing"

func TestConnManagerBasic(t *testing.T) {
	m := NewConnManager("n")
	m.AddPeer("p1", "127.0.0.1:1")
	if _, ok := m.GetPeer("p1"); !ok {
		t.Fatal("missing peer")
	}
	m.RemovePeer("p1")
}
