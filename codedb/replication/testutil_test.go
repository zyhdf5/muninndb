package replication

import (
	"bytes"
	"io"
	"sort"
	"sync"
)

type nopCloser struct{}

func (nopCloser) Close() error { return nil }

type memKVStore struct {
	mu sync.RWMutex
	m  map[string][]byte
}

func newMemKVStore() *memKVStore { return &memKVStore{m: map[string][]byte{}} }

func (s *memKVStore) Get(key []byte) ([]byte, io.Closer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.m[string(key)]
	if !ok {
		return nil, nil, ErrKeyNotFound
	}
	return append([]byte(nil), v...), nopCloser{}, nil
}
func (s *memKVStore) Set(key, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[string(key)] = append([]byte(nil), value...)
	return nil
}
func (s *memKVStore) Delete(key []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, string(key))
	return nil
}
func (s *memKVStore) NewIter(lowerBound, upperBound []byte) (KVIterator, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := make([][]byte, 0, len(s.m))
	for k := range s.m {
		kb := []byte(k)
		if lowerBound != nil && bytes.Compare(kb, lowerBound) < 0 {
			continue
		}
		if upperBound != nil && bytes.Compare(kb, upperBound) >= 0 {
			continue
		}
		keys = append(keys, append([]byte(nil), kb...))
	}
	sort.Slice(keys, func(i, j int) bool { return bytes.Compare(keys[i], keys[j]) < 0 })
	vals := make([][]byte, len(keys))
	for i, k := range keys {
		vals[i] = append([]byte(nil), s.m[string(k)]...)
	}
	return &memIter{keys: keys, vals: vals, idx: -1}, nil
}
func (s *memKVStore) NewBatch() KVBatch { return &memBatch{s: s} }

type memIter struct {
	keys [][]byte
	vals [][]byte
	idx  int
}

func (i *memIter) First() bool {
	if len(i.keys) == 0 {
		i.idx = -1
		return false
	}
	i.idx = 0
	return true
}
func (i *memIter) Next() bool {
	if i.idx < 0 {
		return false
	}
	i.idx++
	return i.idx >= 0 && i.idx < len(i.keys)
}
func (i *memIter) Valid() bool   { return i.idx >= 0 && i.idx < len(i.keys) }
func (i *memIter) Key() []byte   { return i.keys[i.idx] }
func (i *memIter) Value() []byte { return i.vals[i.idx] }
func (i *memIter) Close() error  { return nil }

type batchOp struct {
	set bool
	k   []byte
	v   []byte
}
type memBatch struct {
	s   *memKVStore
	ops []batchOp
}

func (b *memBatch) Set(key, value []byte) error {
	b.ops = append(b.ops, batchOp{set: true, k: append([]byte(nil), key...), v: append([]byte(nil), value...)})
	return nil
}
func (b *memBatch) Delete(key []byte) error {
	b.ops = append(b.ops, batchOp{set: false, k: append([]byte(nil), key...)})
	return nil
}
func (b *memBatch) Commit() error {
	b.s.mu.Lock()
	defer b.s.mu.Unlock()
	for _, op := range b.ops {
		if op.set {
			b.s.m[string(op.k)] = op.v
		} else {
			delete(b.s.m, string(op.k))
		}
	}
	b.ops = nil
	return nil
}
func (b *memBatch) Close() error { return nil }
