package replication

import (
	"context"
	"time"

	"github.com/vmihailenco/msgpack/v5"
)

type Streamer struct {
	log          *ReplicationLog
	entries      chan ReplicationEntry
	done         chan struct{}
	pollInterval time.Duration
}

func NewStreamer(log *ReplicationLog) *Streamer {
	return &Streamer{log: log, entries: make(chan ReplicationEntry, 1024), done: make(chan struct{}), pollInterval: 100 * time.Millisecond}
}

func (s *Streamer) Stream(ctx context.Context, afterSeq uint64) error {
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()
	currentSeq := afterSeq
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.done:
			return nil
		case <-ticker.C:
			entries, err := s.log.ReadSince(currentSeq, 100)
			if err != nil {
				continue
			}
			for _, entry := range entries {
				select {
				case s.entries <- entry:
					currentSeq = entry.Seq
				case <-ctx.Done():
					return ctx.Err()
				case <-s.done:
					return nil
				}
			}
		}
	}
}

func (s *Streamer) Entries() <-chan ReplicationEntry { return s.entries }
func (s *Streamer) Stop() {
	select {
	case <-s.done:
	default:
		close(s.done)
	}
}

type NetworkStreamer struct {
	log     *ReplicationLog
	peer    *PeerConn
	lastSeq uint64
}

func NewNetworkStreamer(log *ReplicationLog, peer *PeerConn, startSeq uint64) *NetworkStreamer {
	return &NetworkStreamer{log: log, peer: peer, lastSeq: startSeq}
}

func (s *NetworkStreamer) Stream(ctx context.Context) error {
	notify, unsub := s.log.Subscribe()
	defer unsub()
	drain := func() error {
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			entries, err := s.log.ReadSince(s.lastSeq, 1000)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				return nil
			}
			for _, entry := range entries {
				payload, err := msgpack.Marshal(ReplEntry{Seq: entry.Seq, Op: uint8(entry.Op), Key: entry.Key, Value: entry.Value, TimestampNS: entry.TimestampNS})
				if err != nil {
					return err
				}
				if err := s.peer.Send(TypeReplEntry, payload); err != nil {
					return err
				}
				s.lastSeq = entry.Seq
			}
		}
	}
	if err := drain(); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-notify:
			if err := drain(); err != nil {
				return err
			}
		}
	}
}
