package replication

import "testing"

func TestReplicationLog(t *testing.T) {
	l := NewReplicationLog(newMemKVStore())
	if _, err := l.Append(OpSet, []byte("k"), []byte("v")); err != nil {
		t.Fatal(err)
	}
	es, err := l.ReadSince(0, 10)
	if err != nil || len(es) != 1 {
		t.Fatal("read")
	}
}
