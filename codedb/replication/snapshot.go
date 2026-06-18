package replication

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/vmihailenco/msgpack/v5"
)

const defaultChunkSize = 1 << 20
const maxBatchSize = 10 * 1024 * 1024
const ackTimeout = 30 * time.Second

var snapCompleteKey = append([]byte{0x19, 0x10}, []byte("snap_complete")...)
var ErrSnapshotInProgress = errors.New("snapshot: transfer already in progress")

type SnapshotSender struct {
	db      KVStore
	repLog  *ReplicationLog
	mu      sync.Mutex
	sending bool
}

func NewSnapshotSender(db KVStore, repLog *ReplicationLog) *SnapshotSender {
	return &SnapshotSender{db: db, repLog: repLog}
}

func (s *SnapshotSender) Send(ctx context.Context, conn *PeerConn) (uint64, error) {
	s.mu.Lock()
	if s.sending {
		s.mu.Unlock()
		return 0, ErrSnapshotInProgress
	}
	s.sending = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.sending = false; s.mu.Unlock() }()

	snapshotSeq := s.repLog.CurrentSeq()
	totalKeys, err := s.countKeys()
	if err != nil {
		return 0, err
	}
	headerPayload, err := msgpack.Marshal(&SnapHeader{SnapshotSeq: snapshotSeq, TotalKeys: totalKeys, Timestamp: time.Now().UnixNano()})
	if err != nil {
		return 0, err
	}
	if err := conn.Send(TypeSnapHeader, headerPayload); err != nil {
		return 0, err
	}
	if err := s.waitForAck(ctx, conn); err != nil {
		return 0, err
	}
	if err := s.streamChunks(ctx, conn); err != nil {
		return 0, err
	}
	if err := conn.Send(TypeSnapComplete, nil); err != nil {
		return 0, err
	}
	return snapshotSeq, nil
}

func (s *SnapshotSender) countKeys() (uint64, error) {
	it, err := s.db.NewIter(nil, nil)
	if err != nil {
		return 0, err
	}
	defer it.Close()
	var c uint64
	for ok := it.First(); ok; ok = it.Next() {
		c++
	}
	return c, nil
}

func (s *SnapshotSender) waitForAck(ctx context.Context, conn *PeerConn) error {
	type res struct {
		t uint8
		e error
	}
	ch := make(chan res, 1)
	go func() { t, _, e := conn.Receive(); ch <- res{t: t, e: e} }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(ackTimeout):
		return errors.New("snapshot: ack timeout")
	case r := <-ch:
		if r.e != nil {
			return r.e
		}
		if r.t != TypeSnapAck {
			return fmt.Errorf("expected snap ack")
		}
		return nil
	}
}

func (s *SnapshotSender) streamChunks(ctx context.Context, conn *PeerConn) error {
	it, err := s.db.NewIter(nil, nil)
	if err != nil {
		return err
	}
	defer it.Close()
	var pairs []KVPair
	chunkNum := uint32(0)
	chunkSize := 0
	send := func(last bool) error {
		payload, err := msgpack.Marshal(&SnapChunk{ChunkNum: chunkNum, LastChunk: last, Pairs: pairs})
		if err != nil {
			return err
		}
		if err := conn.Send(TypeSnapChunk, payload); err != nil {
			return err
		}
		chunkNum++
		pairs = nil
		chunkSize = 0
		return nil
	}
	for ok := it.First(); ok; ok = it.Next() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		k := append([]byte(nil), it.Key()...)
		v := append([]byte(nil), it.Value()...)
		pairs = append(pairs, KVPair{Key: k, Value: v})
		chunkSize += len(k) + len(v)
		if chunkSize >= defaultChunkSize {
			if err := send(false); err != nil {
				return err
			}
		}
	}
	return send(true)
}

type SnapshotReceiver struct{ db KVStore }

func NewSnapshotReceiver(db KVStore) *SnapshotReceiver { return &SnapshotReceiver{db: db} }

func (r *SnapshotReceiver) WipeForResnapshot() error {
	it, err := r.db.NewIter(nil, nil)
	if err != nil {
		return err
	}
	defer it.Close()
	b := r.db.NewBatch()
	defer b.Close()
	for ok := it.First(); ok; ok = it.Next() {
		k := append([]byte(nil), it.Key()...)
		if err := b.Delete(k); err != nil {
			return err
		}
	}
	return b.Commit()
}

func (r *SnapshotReceiver) Receive(ctx context.Context, conn *PeerConn) (uint64, error) {
	if err := r.WipeForResnapshot(); err != nil {
		return 0, err
	}
	t, p, err := conn.Receive()
	if err != nil {
		return 0, err
	}
	if t != TypeSnapHeader {
		return 0, fmt.Errorf("expected snap header")
	}
	var h SnapHeader
	if err := msgpack.Unmarshal(p, &h); err != nil {
		return 0, err
	}
	if err := conn.Send(TypeSnapAck, nil); err != nil {
		return 0, err
	}
	for {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}
		ft, payload, err := conn.Receive()
		if err != nil {
			return 0, err
		}
		if ft == TypeSnapComplete {
			break
		}
		if ft != TypeSnapChunk {
			return 0, fmt.Errorf("expected snap chunk")
		}
		var c SnapChunk
		if err := msgpack.Unmarshal(payload, &c); err != nil {
			return 0, err
		}
		if err := r.applyChunk(c.Pairs, h.SnapshotSeq, c.LastChunk); err != nil {
			return 0, err
		}
	}
	return h.SnapshotSeq, nil
}

func (r *SnapshotReceiver) applyChunk(pairs []KVPair, snapshotSeq uint64, final bool) error {
	b := r.db.NewBatch()
	defer b.Close()
	sz := 0
	for _, kv := range pairs {
		if err := b.Set(kv.Key, kv.Value); err != nil {
			return err
		}
		sz += len(kv.Key) + len(kv.Value)
		if sz >= maxBatchSize {
			if err := b.Commit(); err != nil {
				return err
			}
		}
	}
	if final {
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, snapshotSeq)
		if err := b.Set(snapCompleteKey, buf); err != nil {
			return err
		}
	}
	return b.Commit()
}

func (r *SnapshotReceiver) IsSnapshotComplete() bool {
	_, closer, err := r.db.Get(snapCompleteKey)
	if err != nil {
		return false
	}
	closer.Close()
	return true
}
