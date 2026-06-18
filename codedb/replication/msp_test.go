package replication

import "testing"

func TestMSPBasic(t *testing.T) {
	m := NewMSP("n", "a", NewConnManager("n"))
	m.AddPeer("p", "x", RoleReplica)
	m.HandlePing("p", nil)
	if m.IsSDown("p") {
		t.Fatal("sdown")
	}
}
