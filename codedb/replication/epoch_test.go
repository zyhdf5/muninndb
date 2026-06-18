package replication

import "testing"

func TestEpochStore(t *testing.T) {
	s, err := NewEpochStore(newMemKVStore())
	if err != nil {
		t.Fatal(err)
	}
	if s.Load() != 0 {
		t.Fatal("init")
	}
	ok, err := s.CompareAndSet(0, 1)
	if err != nil || !ok {
		t.Fatal("cas")
	}
}
