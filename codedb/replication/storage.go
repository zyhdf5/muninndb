package replication

import (
	"errors"
	"io"
)

// KVStore 抽象键值存储接口。
type KVStore interface {
	Get(key []byte) ([]byte, io.Closer, error)
	Set(key, value []byte) error
	Delete(key []byte) error
	NewIter(lowerBound, upperBound []byte) (KVIterator, error)
	NewBatch() KVBatch
}

type KVIterator interface {
	First() bool
	Next() bool
	Valid() bool
	Key() []byte
	Value() []byte
	Close() error
}

type KVBatch interface {
	Set(key, value []byte) error
	Delete(key []byte) error
	Commit() error
	Close() error
}

var ErrKeyNotFound = errors.New("key not found")
