package replication

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/vmihailenco/msgpack/v5"
)

var ErrEmptyLog = errors.New("replication: empty log")

const replicationLogPrefix = 0x19

func seqCounterKey() []byte {
	key := make([]byte, 9)
	key[0] = replicationLogPrefix
	for i := 1; i < 9; i++ {
		key[i] = 0xFF
	}
	return key
}

func replicationEntryKey(seq uint64) []byte {
	key := make([]byte, 9)
	key[0] = replicationLogPrefix
	binary.BigEndian.PutUint64(key[1:9], seq)
	return key
}

type ReplicationLog struct {
	db     KVStore
	mu     sync.Mutex
	seq    uint64
	init   bool
	subs   []chan struct{}
	subsMu sync.Mutex
}

func NewReplicationLog(db KVStore) *ReplicationLog { return &ReplicationLog{db: db} }

func (l *ReplicationLog) ensureSeqInit() error {
	if l.init {
		return nil
	}
	val, closer, err := l.db.Get(seqCounterKey())
	if err != nil && err != ErrKeyNotFound {
		return err
	}
	if closer != nil {
		defer closer.Close()
	}
	if err == ErrKeyNotFound || len(val) == 0 {
		l.seq = 0
	} else if len(val) >= 8 {
		l.seq = binary.BigEndian.Uint64(val)
	}
	l.init = true
	return nil
}

func (l *ReplicationLog) Append(op WALOp, key, value []byte) (uint64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.ensureSeqInit(); err != nil {
		return 0, err
	}
	l.seq++
	entry := ReplicationEntry{Seq: l.seq, Op: op, Key: key, Value: value, TimestampNS: timeNowNanos()}
	data, err := msgpack.Marshal(&entry)
	if err != nil {
		l.seq--
		return 0, err
	}
	batch := l.db.NewBatch()
	defer batch.Close()
	if err := batch.Set(replicationEntryKey(l.seq), data); err != nil {
		l.seq--
		return 0, err
	}
	seqBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(seqBuf, l.seq)
	if err := batch.Set(seqCounterKey(), seqBuf); err != nil {
		l.seq--
		return 0, err
	}
	if err := batch.Commit(); err != nil {
		l.seq--
		return 0, err
	}
	seq := l.seq
	l.notifySubscribers()
	return seq, nil
}

func (l *ReplicationLog) ReadSince(afterSeq uint64, limit int) ([]ReplicationEntry, error) {
	l.mu.Lock()
	if err := l.ensureSeqInit(); err != nil {
		l.mu.Unlock()
		return nil, err
	}
	currentSeq := l.seq
	l.mu.Unlock()
	if limit <= 0 {
		limit = 1000
	}
	startKey := replicationEntryKey(afterSeq + 1)
	endKey := seqCounterKey()
	if currentSeq != ^uint64(0) {
		endKey = replicationEntryKey(currentSeq + 1)
	}
	iter, err := l.db.NewIter(startKey, endKey)
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	entries := make([]ReplicationEntry, 0, limit)
	for valid := iter.First(); valid && len(entries) < limit; valid = iter.Next() {
		var entry ReplicationEntry
		if err := msgpack.Unmarshal(iter.Value(), &entry); err != nil {
			var seq uint64
			if key := iter.Key(); len(key) >= 9 {
				seq = binary.BigEndian.Uint64(key[1:9])
			}
			slog.Error("replication log: malformed entry, replication may have gaps", "seq", seq, "err", err)
			return nil, fmt.Errorf("malformed log entry at seq %d: %w", seq, err)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (l *ReplicationLog) Prune(untilSeq uint64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.ensureSeqInit(); err != nil {
		return err
	}
	if untilSeq >= l.seq {
		return nil
	}
	iter, err := l.db.NewIter(replicationEntryKey(1), replicationEntryKey(untilSeq+1))
	if err != nil {
		return err
	}
	defer iter.Close()
	batch := l.db.NewBatch()
	defer batch.Close()
	for valid := iter.First(); valid; valid = iter.Next() {
		k := append([]byte(nil), iter.Key()...)
		if err := batch.Delete(k); err != nil {
			return err
		}
	}
	return batch.Commit()
}

func (l *ReplicationLog) CurrentSeq() uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.ensureSeqInit(); err != nil {
		return 0
	}
	return l.seq
}

func (l *ReplicationLog) Subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	l.subsMu.Lock()
	l.subs = append(l.subs, ch)
	l.subsMu.Unlock()
	unsubscribe := func() {
		l.subsMu.Lock()
		defer l.subsMu.Unlock()
		for i, s := range l.subs {
			if s == ch {
				l.subs = append(l.subs[:i], l.subs[i+1:]...)
				close(ch)
				return
			}
		}
	}
	return ch, unsubscribe
}

func (l *ReplicationLog) notifySubscribers() {
	l.subsMu.Lock()
	defer l.subsMu.Unlock()
	for _, ch := range l.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func timeNowNanos() int64 { return time.Now().UnixNano() }
