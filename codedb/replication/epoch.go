package replication

import (
	"encoding/binary"
	"log/slog"
	"sync"
)

func clusterEpochKey() []byte {
	return []byte{0x19, 0x03, 'c', 'l', 'u', 's', 't', 'e', 'r', '_', 'e', 'p', 'o', 'c', 'h'}
}

func nodeRoleKey() []byte {
	return []byte{0x19, 0x03, 'n', 'o', 'd', 'e', '_', 'r', 'o', 'l', 'e'}
}

type EpochStore struct {
	db      KVStore
	mu      sync.Mutex
	current uint64
}

func NewEpochStore(db KVStore) (*EpochStore, error) {
	s := &EpochStore{db: db}
	val, closer, err := db.Get(clusterEpochKey())
	if err != nil && err != ErrKeyNotFound {
		return nil, err
	}
	if closer != nil {
		defer closer.Close()
	}
	if err == nil && len(val) >= 8 {
		s.current = binary.BigEndian.Uint64(val)
	}
	return s, nil
}

func (s *EpochStore) Load() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

func (s *EpochStore) CompareAndSet(expected, newEpoch uint64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current != expected {
		return false, nil
	}
	if err := s.persist(newEpoch); err != nil {
		return false, err
	}
	s.current = newEpoch
	return true, nil
}

func (s *EpochStore) ForceSet(newEpoch uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if newEpoch <= s.current {
		slog.Debug("epoch store: ignoring stale ForceSet", "current", s.current, "provided", newEpoch)
		return nil
	}
	if err := s.persist(newEpoch); err != nil {
		return err
	}
	s.current = newEpoch
	return nil
}

func (s *EpochStore) persist(epoch uint64) error {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, epoch)
	return s.db.Set(clusterEpochKey(), buf)
}

func (s *EpochStore) PersistRole(role string) error {
	return s.db.Set(nodeRoleKey(), []byte(role))
}

func (s *EpochStore) LoadRole() (string, error) {
	val, closer, err := s.db.Get(nodeRoleKey())
	if err != nil {
		if err == ErrKeyNotFound {
			return "", nil
		}
		return "", err
	}
	defer closer.Close()
	return string(val), nil
}
