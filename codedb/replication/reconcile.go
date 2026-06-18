package replication

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"
)

type HebbianStoreWriter interface {
	UpdateAssocWeight(ctx context.Context, id [16]byte, weight float64) error
}

type ReconcileResult struct {
	StartedAt        time.Time
	CompletedAt      time.Time
	EngramsChecked   int
	EngramsDivergent int
	WeightsSynced    int
	NodeResults      map[string]NodeReconcileResult
}

type NodeReconcileResult struct {
	NodeID    string
	Divergent int
	Synced    int
	Failed    int
	Error     string
}

const reconTimeout = 5 * time.Second
const reconDivergenceThreshold = 1e-6

var ErrReconciliationInProgress = errors.New("reconciliation already in progress")

type Reconciler struct {
	sampler      HebbianSampler
	store        HebbianStoreWriter
	coord        *ClusterCoordinator
	topK         int
	mu           sync.Mutex
	lastResult   ReconcileResult
	running      atomic.Bool
	pendingMu    sync.Mutex
	pendingReply map[string]chan ReconReplyMsg
	pendingAck   map[string]chan ReconAckMsg
}

func NewReconciler(sampler HebbianSampler, store HebbianStoreWriter, coord *ClusterCoordinator) *Reconciler {
	return &Reconciler{sampler: sampler, store: store, coord: coord, topK: 500, pendingReply: map[string]chan ReconReplyMsg{}, pendingAck: map[string]chan ReconAckMsg{}}
}

func (r *Reconciler) LastResult() ReconcileResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastResult
}

func (r *Reconciler) Run(ctx context.Context, lobeNodeIDs []string) (ReconcileResult, error) {
	if !r.running.CompareAndSwap(false, true) {
		return ReconcileResult{}, ErrReconciliationInProgress
	}
	defer r.running.Store(false)
	result := ReconcileResult{StartedAt: time.Now(), NodeResults: map[string]NodeReconcileResult{}}
	if r.sampler == nil || len(lobeNodeIDs) == 0 {
		result.CompletedAt = time.Now()
		r.mu.Lock()
		r.lastResult = result
		r.mu.Unlock()
		return result, nil
	}
	keys, err := r.sampler.SampleKeys(r.topK)
	if err != nil {
		return ReconcileResult{}, fmt.Errorf("reconcile: sample keys: %w", err)
	}
	if len(keys) == 0 {
		result.CompletedAt = time.Now()
		r.mu.Lock()
		r.lastResult = result
		r.mu.Unlock()
		return result, nil
	}
	cortexWeights, err := r.sampler.GetAssocWeightsForKeys(keys)
	if err != nil {
		return ReconcileResult{}, err
	}
	result.EngramsChecked = len(keys)
	weightArr := make([]float64, len(keys))
	for i, k := range keys {
		weightArr[i] = cortexWeights[k]
	}
	rid := uuid.New().String()
	probePayload, err := msgpack.Marshal(ReconProbeMsg{RequestID: rid, Keys: keys, Weights: weightArr})
	if err != nil {
		return ReconcileResult{}, err
	}
	replyCh := make(chan ReconReplyMsg, len(lobeNodeIDs))
	r.pendingMu.Lock()
	r.pendingReply[rid] = replyCh
	r.pendingMu.Unlock()
	defer func() { r.pendingMu.Lock(); delete(r.pendingReply, rid); r.pendingMu.Unlock() }()
	for _, lobeID := range lobeNodeIDs {
		peer, ok := r.coord.mgr.GetPeer(lobeID)
		if !ok {
			result.NodeResults[lobeID] = NodeReconcileResult{NodeID: lobeID, Error: "peer not found"}
			continue
		}
		if err := peer.Send(TypeReconProbe, probePayload); err != nil {
			result.NodeResults[lobeID] = NodeReconcileResult{NodeID: lobeID, Error: err.Error()}
		}
	}
	replies := map[string]ReconReplyMsg{}
	expected := 0
	for _, id := range lobeNodeIDs {
		if _, bad := result.NodeResults[id]; !bad {
			expected++
		}
	}
	wctx, cancel := context.WithTimeout(ctx, reconTimeout)
	defer cancel()
	for len(replies) < expected {
		select {
		case <-wctx.Done():
			goto reconcile
		case rep := <-replyCh:
			replies[rep.NodeID] = rep
		}
	}
reconcile:
	totalDiv := 0
	for _, lobeID := range lobeNodeIDs {
		if _, bad := result.NodeResults[lobeID]; bad {
			continue
		}
		rep, ok := replies[lobeID]
		if !ok {
			result.NodeResults[lobeID] = NodeReconcileResult{NodeID: lobeID, Error: "timeout waiting for reply"}
			continue
		}
		var dKeys [][16]byte
		var dWeights []float64
		for i, k := range keys {
			lw := 0.0
			if i < len(rep.Weights) {
				lw = rep.Weights[i]
			}
			if math.Abs(weightArr[i]-lw) > reconDivergenceThreshold {
				dKeys = append(dKeys, k)
				dWeights = append(dWeights, weightArr[i])
			}
		}
		nr := NodeReconcileResult{NodeID: lobeID, Divergent: len(dKeys)}
		totalDiv += len(dKeys)
		if len(dKeys) == 0 {
			result.NodeResults[lobeID] = nr
			continue
		}
		syncRID := uuid.New().String()
		syncPayload, err := msgpack.Marshal(ReconSyncMsg{RequestID: syncRID, Keys: dKeys, Weights: dWeights})
		if err != nil {
			nr.Error = err.Error()
			nr.Failed = len(dKeys)
			result.NodeResults[lobeID] = nr
			continue
		}
		ackCh := make(chan ReconAckMsg, 1)
		r.pendingMu.Lock()
		r.pendingAck[syncRID] = ackCh
		r.pendingMu.Unlock()
		peer, ok := r.coord.mgr.GetPeer(lobeID)
		if !ok {
			nr.Error = "peer not found for sync"
			nr.Failed = len(dKeys)
			r.pendingMu.Lock()
			delete(r.pendingAck, syncRID)
			r.pendingMu.Unlock()
			result.NodeResults[lobeID] = nr
			continue
		}
		if err := peer.Send(TypeReconSync, syncPayload); err != nil {
			nr.Error = err.Error()
			nr.Failed = len(dKeys)
			r.pendingMu.Lock()
			delete(r.pendingAck, syncRID)
			r.pendingMu.Unlock()
			result.NodeResults[lobeID] = nr
			continue
		}
		tm := time.NewTimer(reconTimeout)
		select {
		case ack := <-ackCh:
			applied := ack.Applied
			if applied > len(dKeys) {
				applied = len(dKeys)
			}
			nr.Synced = applied
			nr.Failed = len(dKeys) - applied
		case <-tm.C:
			nr.Error = "timeout waiting for sync ack"
			nr.Failed = len(dKeys)
		case <-ctx.Done():
			nr.Error = "context cancelled"
			nr.Failed = len(dKeys)
		}
		tm.Stop()
		r.pendingMu.Lock()
		delete(r.pendingAck, syncRID)
		r.pendingMu.Unlock()
		result.NodeResults[lobeID] = nr
	}
	result.EngramsDivergent = totalDiv
	for _, nr := range result.NodeResults {
		result.WeightsSynced += nr.Synced
	}
	result.CompletedAt = time.Now()
	r.mu.Lock()
	r.lastResult = result
	r.mu.Unlock()
	return result, nil
}

func (r *Reconciler) HandleReconReply(fromNodeID string, payload []byte) error {
	var reply ReconReplyMsg
	if err := msgpack.Unmarshal(payload, &reply); err != nil {
		return fmt.Errorf("reconcile: unmarshal ReconReply: %w", err)
	}
	r.pendingMu.Lock()
	ch, ok := r.pendingReply[reply.RequestID]
	if ok {
		select {
		case ch <- reply:
		default:
		}
	}
	r.pendingMu.Unlock()
	_ = fromNodeID
	return nil
}

func (r *Reconciler) HandleReconAck(fromNodeID string, payload []byte) error {
	var ack ReconAckMsg
	if err := msgpack.Unmarshal(payload, &ack); err != nil {
		return fmt.Errorf("reconcile: unmarshal ReconAck: %w", err)
	}
	r.pendingMu.Lock()
	ch, ok := r.pendingAck[ack.RequestID]
	if ok {
		select {
		case ch <- ack:
		default:
		}
	}
	r.pendingMu.Unlock()
	_ = fromNodeID
	return nil
}

func (r *Reconciler) HandleReconProbe(fromNodeID string, payload []byte) error {
	var probe ReconProbeMsg
	if err := msgpack.Unmarshal(payload, &probe); err != nil {
		return fmt.Errorf("reconcile: unmarshal ReconProbe: %w", err)
	}
	localWeights := make([]float64, len(probe.Keys))
	if r.sampler != nil {
		wm, err := r.sampler.GetAssocWeightsForKeys(probe.Keys)
		if err == nil {
			for i, k := range probe.Keys {
				localWeights[i] = wm[k]
			}
		}
	}
	replyPayload, err := msgpack.Marshal(ReconReplyMsg{RequestID: probe.RequestID, NodeID: r.coord.cfg.NodeID, Weights: localWeights})
	if err != nil {
		return err
	}
	peer, ok := r.coord.mgr.GetPeer(fromNodeID)
	if !ok {
		return nil
	}
	return peer.Send(TypeReconReply, replyPayload)
}

func (r *Reconciler) HandleReconSync(fromNodeID string, payload []byte) error {
	var syncMsg ReconSyncMsg
	if err := msgpack.Unmarshal(payload, &syncMsg); err != nil {
		return fmt.Errorf("reconcile: unmarshal ReconSync: %w", err)
	}
	applied := 0
	if r.store != nil {
		for i, k := range syncMsg.Keys {
			if i >= len(syncMsg.Weights) {
				break
			}
			if err := r.store.UpdateAssocWeight(context.Background(), k, syncMsg.Weights[i]); err == nil {
				applied++
			}
		}
	}
	ackPayload, err := msgpack.Marshal(ReconAckMsg{RequestID: syncMsg.RequestID, NodeID: r.coord.cfg.NodeID, Applied: applied})
	if err != nil {
		return err
	}
	peer, ok := r.coord.mgr.GetPeer(fromNodeID)
	if !ok {
		return nil
	}
	return peer.Send(TypeReconAck, ackPayload)
}
