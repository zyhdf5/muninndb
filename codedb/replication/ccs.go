package replication

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"
)

type HebbianSampler interface {
	SampleKeys(n int) ([][16]byte, error)
	GetAssocWeightsForKeys(keys [][16]byte) (map[[16]byte]float64, error)
}

type CCSResult struct {
	Score      float64            `json:"score"`
	Assessment string             `json:"assessment"`
	NodeScores map[string]float64 `json:"node_scores"`
	SampledAt  time.Time          `json:"sampled_at"`
}

func ccsAssessment(score float64) string {
	switch {
	case score > 0.99:
		return "excellent"
	case score > 0.95:
		return "good"
	case score > 0.90:
		return "degraded"
	default:
		return "critical"
	}
}

type CCSProbe struct {
	store          HebbianSampler
	coord          *ClusterCoordinator
	mu             sync.RWMutex
	last           CCSResult
	sampleN        int
	probeIntervalS atomic.Int32
	pendingMu      sync.Mutex
	pending        map[string]chan CCSResponseMsg
}

const defaultCCSIntervalS = 30

func NewCCSProbe(store HebbianSampler, coord *ClusterCoordinator) *CCSProbe {
	p := &CCSProbe{store: store, coord: coord, sampleN: 100, pending: map[string]chan CCSResponseMsg{}, last: CCSResult{Score: 1.0, Assessment: "excellent", NodeScores: map[string]float64{}, SampledAt: time.Now()}}
	p.probeIntervalS.Store(defaultCCSIntervalS)
	return p
}

func (p *CCSProbe) LastResult() CCSResult { p.mu.RLock(); defer p.mu.RUnlock(); return p.last }
func (p *CCSProbe) SetInterval(d time.Duration) {
	s := int32(d.Seconds())
	if s < 1 {
		s = 1
	}
	p.probeIntervalS.Store(s)
}

func (p *CCSProbe) Run(ctx context.Context) {
	interval := time.Duration(p.probeIntervalS.Load()) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		newInterval := time.Duration(p.probeIntervalS.Load()) * time.Second
		if newInterval != interval {
			interval = newInterval
			ticker.Reset(interval)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if p.coord.IsLeader() {
				p.probe(ctx)
			}
		}
	}
}

func (p *CCSProbe) probe(ctx context.Context) {
	lobes := p.coord.joinHandler.Members()
	if len(lobes) == 0 {
		p.mu.Lock()
		p.last = CCSResult{Score: 1.0, Assessment: "excellent", NodeScores: map[string]float64{}, SampledAt: time.Now()}
		p.mu.Unlock()
		return
	}
	var sampledKeys [][16]byte
	if p.store != nil {
		k, err := p.store.SampleKeys(p.sampleN)
		if err != nil {
			return
		}
		sampledKeys = k
	}
	localHash, err := p.computeLocalHash(sampledKeys)
	if err != nil {
		return
	}
	rid := uuid.New().String()
	payload, err := msgpack.Marshal(CCSProbeMsg{SampledKeys: sampledKeys, RequestID: rid})
	if err != nil {
		return
	}
	p.pendingMu.Lock()
	p.pending[rid] = make(chan CCSResponseMsg, len(lobes))
	ch := p.pending[rid]
	p.pendingMu.Unlock()
	defer func() { p.pendingMu.Lock(); delete(p.pending, rid); p.pendingMu.Unlock() }()
	for _, l := range lobes {
		if peer, ok := p.coord.mgr.GetPeer(l.NodeID); ok {
			_ = peer.Send(TypeCCSProbe, payload)
		}
	}
	responses := map[string]CCSResponseMsg{}
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for len(responses) < len(lobes) {
		select {
		case <-waitCtx.Done():
			goto compute
		case resp := <-ch:
			responses[resp.NodeID] = resp
		}
	}
compute:
	nodeScores := map[string]float64{}
	matched := 0
	for _, l := range lobes {
		resp, ok := responses[l.NodeID]
		if !ok {
			nodeScores[l.NodeID] = 0
			continue
		}
		if hashesEqual(localHash, resp.Hash) {
			nodeScores[l.NodeID] = 1
			matched++
		} else {
			nodeScores[l.NodeID] = 0
		}
	}
	score := float64(matched) / float64(len(lobes))
	res := CCSResult{Score: score, Assessment: ccsAssessment(score), NodeScores: nodeScores, SampledAt: time.Now()}
	p.mu.Lock()
	p.last = res
	p.mu.Unlock()
}

func (p *CCSProbe) HandleCCSResponse(fromNodeID string, payload []byte) error {
	var resp CCSResponseMsg
	if err := msgpack.Unmarshal(payload, &resp); err != nil {
		return err
	}
	p.pendingMu.Lock()
	ch, ok := p.pending[resp.RequestID]
	if ok {
		select {
		case ch <- resp:
		default:
		}
	}
	p.pendingMu.Unlock()
	_ = fromNodeID
	return nil
}

func (p *CCSProbe) HandleCCSProbe(fromNodeID string, payload []byte) error {
	var probe CCSProbeMsg
	if err := msgpack.Unmarshal(payload, &probe); err != nil {
		return err
	}
	hash := []byte{}
	if p.store != nil {
		if h, err := p.computeLocalHash(probe.SampledKeys); err == nil {
			hash = h
		}
	}
	respPayload, err := msgpack.Marshal(CCSResponseMsg{RequestID: probe.RequestID, NodeID: p.coord.cfg.NodeID, Hash: hash, KeyCount: len(probe.SampledKeys)})
	if err != nil {
		return err
	}
	peer, ok := p.coord.mgr.GetPeer(fromNodeID)
	if !ok {
		return nil
	}
	return peer.Send(TypeCCSResponse, respPayload)
}

func (p *CCSProbe) computeLocalHash(keys [][16]byte) ([]byte, error) {
	if len(keys) == 0 {
		h := sha256.New()
		return h.Sum(nil), nil
	}
	weights := map[[16]byte]float64{}
	if p.store != nil {
		m, err := p.store.GetAssocWeightsForKeys(keys)
		if err != nil {
			return nil, err
		}
		weights = m
	}
	return ComputeCCSHash(keys, weights), nil
}

func ComputeCCSHash(keys [][16]byte, weights map[[16]byte]float64) []byte {
	sorted := make([][16]byte, len(keys))
	copy(sorted, keys)
	sort.Slice(sorted, func(i, j int) bool {
		for b := 0; b < 16; b++ {
			if sorted[i][b] != sorted[j][b] {
				return sorted[i][b] < sorted[j][b]
			}
		}
		return false
	})
	h := sha256.New()
	var buf [24]byte
	for _, k := range sorted {
		copy(buf[:16], k[:])
		binary.BigEndian.PutUint64(buf[16:], math.Float64bits(weights[k]))
		h.Write(buf[:])
	}
	return h.Sum(nil)
}

func hashesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
