package replication

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"sync"
)

func lastAppliedKey() []byte {
	return []byte{0x19, 0x02, 'l', 'a', 's', 't', '_', 'a', 'p', 'p'}
}

type Applier struct {
	db          KVStore
	lastApplied uint64
	mu          sync.Mutex
}

func NewApplier(db KVStore) *Applier {
	a := &Applier{db: db}
	val, closer, err := db.Get(lastAppliedKey())
	if err == nil && len(val) >= 8 {
		a.lastApplied = binary.BigEndian.Uint64(val)
	}
	if closer != nil {
		closer.Close()
	}
	return a
}

func (a *Applier) Apply(entry ReplicationEntry) (returnErr error) {
	defer func() {
		if r := recover(); r != nil {
			returnErr = fmt.Errorf("applier panic: %v", r)
			slog.Error("applier: panic recovered", "panic", r)
		}
	}()

	a.mu.Lock()
	defer a.mu.Unlock()
	if entry.Seq <= a.lastApplied {
		return nil
	}
	batch := a.db.NewBatch()
	defer batch.Close()
	switch entry.Op {
	case OpDelete:
		if err := batch.Delete(entry.Key); err != nil {
			return err
		}
	default:
		if err := batch.Set(entry.Key, entry.Value); err != nil {
			return err
		}
	}
	seqBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(seqBuf, entry.Seq)
	if err := batch.Set(lastAppliedKey(), seqBuf); err != nil {
		return err
	}
	if err := batch.Commit(); err != nil {
		return err
	}
	a.lastApplied = entry.Seq
	return nil
}

func (a *Applier) LastApplied() uint64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lastApplied
}

func (a *Applier) IsLagging(primarySeq uint64, maxLag uint64) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.lastApplied >= primarySeq {
		return false
	}
	return (primarySeq - a.lastApplied) > maxLag
}
