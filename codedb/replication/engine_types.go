package replication

import "time"

type CoActivatedEngram struct {
	ID    [16]byte
	Score float64
}

type CoActivationEvent struct {
	WS      [8]byte
	At      time.Time
	Engrams []CoActivatedEngram
}

type WALPruner interface {
	PruneApplied(seq uint64) error
}
