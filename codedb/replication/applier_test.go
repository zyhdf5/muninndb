package replication

import "testing"

func TestApplier(t *testing.T) {
	db := newMemKVStore()
	a := NewApplier(db)
	if err := a.Apply(ReplicationEntry{Seq: 1, Op: OpSet, Key: []byte("k"), Value: []byte("v")}); err != nil {
		t.Fatal(err)
	}
	if a.LastApplied() != 1 {
		t.Fatal("last applied")
	}
}
