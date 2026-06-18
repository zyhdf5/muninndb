package replication

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vmihailenco/msgpack/v5"
)

type hebbianSubmitter interface {
	Submit(item CoActivationEvent) bool
}
type cognitiveFlushable interface{ Stop() }

type NodeState uint8

const (
	StateNormal NodeState = iota
	StateDraining
)

var ErrDraining = errors.New("node is draining: not accepting new writes")
var ErrSelfRemoval = errors.New("cannot remove self from cluster")

type ClusterCoordinator struct {
	cfg                *ClusterConfig
	repLog             *ReplicationLog
	applier            *Applier
	epochStore         *EpochStore
	mgr                *ConnManager
	msp                *MSP
	election           *Election
	joinHandler        *JoinHandler
	joinClient         *JoinClient
	tokenManager       *JoinTokenManager
	tls                *ClusterTLS
	role               NodeRole
	roleMu             sync.RWMutex
	streamers          map[string]context.CancelFunc
	streamersMu        sync.Mutex
	quorumLostSince    time.Time
	quorumMu           sync.Mutex
	hebbianWorker      hebbianSubmitter
	hebbianFlusher     cognitiveFlushable
	cogForwardedTotal  uint64
	nodeState          atomic.Uint32
	reconcileOnHeal    atomic.Uint32
	ccsProbe           *CCSProbe
	mol                WALPruner
	snapshotInProgress atomic.Int32
	reconciler         *Reconciler
	reconDelay         time.Duration
	failoverMu         sync.Mutex
	replicaSeqs        sync.Map
	handoffAckCh       chan HandoffAck
	handoffMu          sync.Mutex
	started            atomic.Bool
	OnBecameCortex     func(epoch uint64)
	OnBecameLobe       func()
}

type Coordinator = ClusterCoordinator

func NewClusterCoordinator(cfg *ClusterConfig, repLog *ReplicationLog, applier *Applier, epochStore *EpochStore) *ClusterCoordinator {
	mgr := NewConnManager(cfg.NodeID)
	msp := NewMSP(cfg.NodeID, cfg.BindAddr, mgr)
	election := NewElection(cfg.NodeID, epochStore, mgr)
	joinHandler := NewJoinHandler(cfg.NodeID, cfg.ClusterSecret, epochStore, repLog, mgr)
	joinClient := NewJoinClient(cfg.NodeID, cfg.BindAddr, cfg.ClusterSecret, epochStore, applier, mgr)
	reconDelay := time.Duration(cfg.ReconDelayMs) * time.Millisecond
	if reconDelay <= 0 {
		reconDelay = 2 * time.Second
	}
	c := &ClusterCoordinator{cfg: cfg, repLog: repLog, applier: applier, epochStore: epochStore, mgr: mgr, msp: msp, election: election, joinHandler: joinHandler, joinClient: joinClient, role: RoleUnknown, streamers: map[string]context.CancelFunc{}, reconDelay: reconDelay}
	c.reconcileOnHeal.Store(1)
	if cfg.ClusterSecret != "" {
		ttl := time.Duration(cfg.JoinTokenTTLMin) * time.Minute
		if ttl <= 0 {
			ttl = 15 * time.Minute
		}
		c.tokenManager = NewJoinTokenManager(cfg.ClusterSecret, ttl)
	}
	election.OnPromoted = func(epoch uint64) { c.handlePromotion(epoch) }
	election.OnDemoted = func() { c.handleDemotion() }
	election.OnNewLeader = func(leaderID string, epoch uint64) { c.handleNewLeader(leaderID, epoch) }
	msp.OnODown = func(nodeID string) {
		if c.IsSentinel() {
			return
		}
		_ = c.election.StartElection(context.Background())
		_ = nodeID
	}
	msp.OnSDown = func(nodeID string) { c.checkQuorumHealth(); _ = nodeID }
	msp.OnRecover = func(nodeID string) {
		if !c.IsLeader() {
			return
		}
		for _, p := range c.msp.AllPeers() {
			if p.NodeID == nodeID {
				c.startStreamerForLobe(NodeInfo{NodeID: p.NodeID, Addr: p.Addr, Role: p.Role})
				if c.reconciler != nil && c.reconcileOnHeal.Load() == 1 {
					go func() {
						time.Sleep(c.reconDelay)
						if c.IsLeader() {
							_, _ = c.TriggerReconciliation(context.Background(), []string{nodeID})
						}
					}()
				}
				return
			}
		}
	}
	msp.OnAddrChanged = func(nodeID, newAddr string) { c.mgr.AddPeer(nodeID, newAddr) }
	joinHandler.OnLobeJoined = func(info NodeInfo) { c.startStreamerForLobe(info) }
	joinHandler.OnLobeLeft = func(nodeID string) { c.stopStreamerForLobe(nodeID) }
	return c
}

func (c *ClusterCoordinator) Run(ctx context.Context) error {
	c.started.Store(true)
	if c.cfg.Role != "observer" {
		c.election.RegisterVoter(c.cfg.NodeID)
	} else {
		c.election.SetObserver(true)
	}
	for _, seed := range c.cfg.Seeds {
		seedID := "seed-" + seed
		c.mgr.AddPeer(seedID, seed)
		c.msp.AddPeer(seedID, seed, RoleUnknown)
		if c.cfg.Role != "observer" {
			c.election.RegisterVoter(seedID)
		}
	}
	heartbeat := time.Duration(c.cfg.HeartbeatMS) * time.Millisecond
	if heartbeat <= 0 {
		heartbeat = time.Second
	}
	mspCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	missed := c.cfg.SDOWNBeats
	if missed <= 0 {
		missed = 3
	}
	go func() { _ = c.msp.Run(mspCtx, heartbeat, missed) }()
	c.startPeriodicPrune(ctx)
	switch c.cfg.Role {
	case "primary":
		return c.runAsCortex(ctx)
	case "replica":
		return c.runAsLobe(ctx)
	case "sentinel":
		return c.runAsSentinel(ctx)
	case "observer":
		return c.runAsObserver(ctx)
	default:
		return c.runAsCortex(ctx)
	}
}

func (c *ClusterCoordinator) runAsCortex(ctx context.Context) error {
	if c.epochStore.Load() == 0 {
		if err := c.election.StartElection(ctx); err != nil {
			return fmt.Errorf("cluster: bootstrap election failed: %w", err)
		}
	}
	<-ctx.Done()
	return ctx.Err()
}

func (c *ClusterCoordinator) joinWithRetry(ctx context.Context, seeds []string, role string) (JoinResult, error) {
	_ = role
	if len(seeds) == 0 {
		return JoinResult{}, errors.New("no seeds")
	}
	for {
		for _, addr := range seeds {
			res, err := c.joinClient.Join(ctx, addr)
			if err == nil {
				return res, nil
			}
		}
		select {
		case <-ctx.Done():
			return JoinResult{}, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func (c *ClusterCoordinator) runAsLobe(ctx context.Context) error {
	c.roleMu.Lock()
	c.role = RoleReplica
	c.roleMu.Unlock()
	if len(c.cfg.Seeds) == 0 {
		return errors.New("cluster: lobe requires at least one seed address")
	}
	_, err := c.joinWithRetry(ctx, c.cfg.Seeds, "lobe")
	if err != nil {
		return err
	}
	<-ctx.Done()
	return ctx.Err()
}
func (c *ClusterCoordinator) runAsSentinel(ctx context.Context) error {
	c.roleMu.Lock()
	c.role = RoleSentinel
	c.roleMu.Unlock()
	c.election.SetSentinel(true)
	<-ctx.Done()
	return ctx.Err()
}
func (c *ClusterCoordinator) runAsObserver(ctx context.Context) error {
	c.roleMu.Lock()
	c.role = RoleObserver
	c.roleMu.Unlock()
	if len(c.cfg.Seeds) == 0 {
		return errors.New("cluster: observer requires at least one seed address")
	}
	_, err := c.joinWithRetry(ctx, c.cfg.Seeds, "observer")
	if err != nil {
		return err
	}
	<-ctx.Done()
	return ctx.Err()
}

func (c *ClusterCoordinator) IsObserver() bool { return c.Role() == RoleObserver }
func (c *ClusterCoordinator) Role() NodeRole {
	c.roleMu.RLock()
	defer c.roleMu.RUnlock()
	return c.role
}
func (c *ClusterCoordinator) IsLeader() bool       { return c.Role() == RolePrimary }
func (c *ClusterCoordinator) IsSentinel() bool     { return c.Role() == RoleSentinel }
func (c *ClusterCoordinator) CurrentEpoch() uint64 { return c.epochStore.Load() }
func (c *ClusterCoordinator) KnownNodes() []NodeInfo {
	peers := c.msp.AllPeers()
	nodes := []NodeInfo{{NodeID: c.cfg.NodeID, Addr: c.cfg.BindAddr, Role: c.Role()}}
	for _, p := range peers {
		nodes = append(nodes, NodeInfo{NodeID: p.NodeID, Addr: p.Addr, Role: p.Role})
	}
	return nodes
}
func (c *ClusterCoordinator) ReplicationLag() uint64 {
	if c.IsLeader() {
		return 0
	}
	cs := c.repLog.CurrentSeq()
	la := c.applier.LastApplied()
	if cs <= la {
		return 0
	}
	return cs - la
}
func (c *ClusterCoordinator) CortexID() string           { return c.election.CurrentLeader() }
func (c *ClusterCoordinator) FencingToken() uint64       { return c.epochStore.Load() }
func (c *ClusterCoordinator) ClusterMembers() []NodeInfo { return c.KnownNodes() }

func (c *ClusterCoordinator) checkQuorumHealth() {
	if !c.IsLeader() {
		return
	}
	live := c.msp.LivePeers()
	quorum := c.election.Quorum()
	total := 1 + len(live)
	c.quorumMu.Lock()
	if total >= quorum {
		c.quorumLostSince = time.Time{}
		c.quorumMu.Unlock()
		return
	}
	now := time.Now()
	if c.quorumLostSince.IsZero() {
		c.quorumLostSince = now
		c.quorumMu.Unlock()
		return
	}
	qt := time.Duration(c.cfg.QuorumLossTimeoutSec) * time.Second
	if qt <= 0 {
		qt = 5 * time.Second
	}
	need := now.Sub(c.quorumLostSince) >= qt
	if need {
		c.quorumLostSince = time.Time{}
	}
	c.quorumMu.Unlock()
	if need {
		c.nodeState.Store(uint32(StateDraining))
		go c.handleDemotion()
	}
}

func (c *ClusterCoordinator) HandleIncomingFrame(fromNodeID string, frameType uint8, payload []byte) error {
	switch frameType {
	case TypePing:
		c.msp.HandlePing(fromNodeID, payload)
		return nil
	case TypePong:
		c.msp.HandlePong(fromNodeID, payload)
		return nil
	case TypeVoteRequest:
		var req VoteRequest
		if err := msgpack.Unmarshal(payload, &req); err != nil {
			return err
		}
		resp := c.election.HandleVoteRequest(req)
		p, _ := msgpack.Marshal(resp)
		if peer, ok := c.mgr.GetPeer(fromNodeID); ok {
			_ = peer.Send(TypeVoteResponse, p)
		}
		return nil
	case TypeVoteResponse:
		var resp VoteResponse
		if err := msgpack.Unmarshal(payload, &resp); err != nil {
			return err
		}
		c.election.HandleVoteResponse(resp)
		return nil
	case TypeCortexClaim:
		var claim CortexClaim
		if err := msgpack.Unmarshal(payload, &claim); err != nil {
			return err
		}
		c.election.HandleCortexClaim(claim)
		return nil
	case TypeJoinRequest:
		var req JoinRequest
		if err := msgpack.Unmarshal(payload, &req); err != nil {
			return err
		}
		peer, ok := c.mgr.GetPeer(fromNodeID)
		if !ok {
			c.mgr.AddPeer(req.NodeID, req.Addr)
			peer, ok = c.mgr.GetPeer(req.NodeID)
			if !ok {
				return errors.New("failed to create peer")
			}
		}
		resp := c.joinHandler.HandleJoinRequest(req, peer)
		p, _ := msgpack.Marshal(resp)
		_ = peer.Send(TypeJoinResponse, p)
		if resp.NeedsSnapshot {
			c.IncrementSnapshotCount()
			go func() {
				defer c.DecrementSnapshotCount()
				_, _ = c.joinHandler.StreamSnapshot(context.Background(), peer)
			}()
		}
		return nil
	case TypeLeave:
		var msg LeaveMessage
		if err := msgpack.Unmarshal(payload, &msg); err != nil {
			return err
		}
		c.joinHandler.HandleLeave(msg)
		return nil
	case TypeReplEntry:
		if c.IsSentinel() {
			return nil
		}
		var e ReplEntry
		if err := msgpack.Unmarshal(payload, &e); err != nil {
			return err
		}
		return c.applier.Apply(ReplicationEntry{Seq: e.Seq, Op: WALOp(e.Op), Key: e.Key, Value: e.Value, TimestampNS: e.TimestampNS})
	case TypeReplAck:
		var ack ReplAck
		if err := msgpack.Unmarshal(payload, &ack); err != nil {
			return err
		}
		c.UpdateReplicaSeq(ack.NodeID, ack.LastSeq)
		return nil
	case TypeCogForward:
		return c.handleCogForward(fromNodeID, payload)
	case TypeHandoff:
		return c.HandleHandoff(fromNodeID, payload)
	case TypeHandoffAck:
		return c.HandleHandoffAck(fromNodeID, payload)
	case TypeCCSProbe:
		if c.ccsProbe != nil {
			return c.ccsProbe.HandleCCSProbe(fromNodeID, payload)
		}
		return nil
	case TypeCCSResponse:
		if c.ccsProbe != nil {
			return c.ccsProbe.HandleCCSResponse(fromNodeID, payload)
		}
		return nil
	case TypeReconProbe:
		if c.reconciler != nil {
			return c.reconciler.HandleReconProbe(fromNodeID, payload)
		}
		return nil
	case TypeReconReply:
		if c.reconciler != nil {
			return c.reconciler.HandleReconReply(fromNodeID, payload)
		}
		return nil
	case TypeReconSync:
		if c.reconciler != nil {
			return c.reconciler.HandleReconSync(fromNodeID, payload)
		}
		return nil
	case TypeReconAck:
		if c.reconciler != nil {
			return c.reconciler.HandleReconAck(fromNodeID, payload)
		}
		return nil
	default:
		return fmt.Errorf("unknown frame type: 0x%02x", frameType)
	}
}

func (c *ClusterCoordinator) Stop() error {
	c.streamersMu.Lock()
	for id, cancel := range c.streamers {
		cancel()
		delete(c.streamers, id)
	}
	c.streamersMu.Unlock()
	return c.mgr.Close()
}

func (c *ClusterCoordinator) handlePromotion(epoch uint64) {
	c.election.mu.Lock()
	if c.election.state != ElectionLeader {
		c.election.mu.Unlock()
		return
	}
	c.roleMu.Lock()
	c.role = RolePrimary
	c.roleMu.Unlock()
	c.election.mu.Unlock()
	if c.OnBecameCortex != nil {
		c.OnBecameCortex(epoch)
	}
}

func (c *ClusterCoordinator) handleDemotion() {
	c.roleMu.Lock()
	c.role = RoleReplica
	c.roleMu.Unlock()
	c.streamersMu.Lock()
	for id, cancel := range c.streamers {
		cancel()
		delete(c.streamers, id)
	}
	c.streamersMu.Unlock()
	c.nodeState.Store(uint32(StateNormal))
	if c.OnBecameLobe != nil {
		c.OnBecameLobe()
	}
}
func (c *ClusterCoordinator) handleNewLeader(leaderID string, epoch uint64) { _, _ = leaderID, epoch }

func (c *ClusterCoordinator) startStreamerForLobe(info NodeInfo) {
	peer, ok := c.mgr.GetPeer(info.NodeID)
	if !ok {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.streamersMu.Lock()
	if ex, ok := c.streamers[info.NodeID]; ok {
		ex()
	}
	c.streamers[info.NodeID] = cancel
	c.streamersMu.Unlock()
	go func() { _ = NewNetworkStreamer(c.repLog, peer, 0).Stream(ctx) }()
}
func (c *ClusterCoordinator) stopStreamerForLobe(nodeID string) {
	c.streamersMu.Lock()
	if cancel, ok := c.streamers[nodeID]; ok {
		cancel()
		delete(c.streamers, nodeID)
	}
	c.streamersMu.Unlock()
}

func (c *ClusterCoordinator) ForwardCognitiveEffects(effect CognitiveSideEffect) {
	if c.IsObserver() {
		return
	}
	go func() {
		if peer, ok := c.mgr.GetPeer(c.election.CurrentLeader()); ok {
			if p, err := msgpack.Marshal(effect); err == nil {
				_ = peer.Send(TypeCogForward, p)
			}
		}
	}()
}

func (c *ClusterCoordinator) ConnManager() *ConnManager           { return c.mgr }
func (c *ClusterCoordinator) MSP() *MSP                           { return c.msp }
func (c *ClusterCoordinator) Election() *Election                 { return c.election }
func (c *ClusterCoordinator) RepLog() *ReplicationLog             { return c.repLog }
func (c *ClusterCoordinator) JoinTokenManager() *JoinTokenManager { return c.tokenManager }
func (c *ClusterCoordinator) ClusterSecret() string {
	if c.cfg == nil {
		return ""
	}
	return c.cfg.ClusterSecret
}
func (c *ClusterCoordinator) TLSManager() *ClusterTLS { return c.tls }
func (c *ClusterCoordinator) SetTLSManager(t *ClusterTLS) {
	if c.started.Load() {
		panic("SetTLSManager called after Run()")
	}
	c.tls = t
}
func (c *ClusterCoordinator) SetCognitiveWorkers(hebbian hebbianSubmitter) {
	if c.started.Load() {
		panic("SetCognitiveWorkers called after Run()")
	}
	c.hebbianWorker = hebbian
}
func (c *ClusterCoordinator) CogForwardedTotal() uint64 {
	return atomic.LoadUint64(&c.cogForwardedTotal)
}

func (c *ClusterCoordinator) handleCogForward(fromNodeID string, payload []byte) error {
	var effect CognitiveSideEffect
	if err := msgpack.Unmarshal(payload, &effect); err != nil {
		return err
	}
	if c.hebbianWorker != nil && len(effect.CoActivations) > 0 {
		e := make([]CoActivatedEngram, len(effect.CoActivations))
		for i, ca := range effect.CoActivations {
			e[i] = CoActivatedEngram{ID: ca.ID, Score: ca.Score}
		}
		c.hebbianWorker.Submit(CoActivationEvent{WS: [8]byte{}, At: time.Unix(0, effect.Timestamp), Engrams: e})
	}
	if n := uint64(len(effect.CoActivations)); n > 0 {
		atomic.AddUint64(&c.cogForwardedTotal, n)
	}
	if n := uint64(len(effect.RestoredEdges)); n > 0 {
		atomic.AddUint64(&c.cogForwardedTotal, n)
	}
	ackPayload, err := msgpack.Marshal(CogAck{QueryID: effect.QueryID})
	if err == nil {
		if peer, ok := c.mgr.GetPeer(fromNodeID); ok {
			_ = peer.Send(TypeCogAck, ackPayload)
		}
	}
	return nil
}

func (c *ClusterCoordinator) IsDraining() bool { return NodeState(c.nodeState.Load()) == StateDraining }
func (c *ClusterCoordinator) SetCognitiveFlushers(hebbian cognitiveFlushable) {
	if c.started.Load() {
		panic("SetCognitiveFlushers called after Run()")
	}
	c.hebbianFlusher = hebbian
}
func (c *ClusterCoordinator) SetCCSProbe(probe *CCSProbe) {
	if c.started.Load() {
		panic("SetCCSProbe called after Run()")
	}
	c.ccsProbe = probe
}
func (c *ClusterCoordinator) CognitiveConsistency() CCSResult {
	if c.ccsProbe == nil {
		return CCSResult{Score: 1, Assessment: "excellent", NodeScores: map[string]float64{}, SampledAt: time.Now()}
	}
	return c.ccsProbe.LastResult()
}
func (c *ClusterCoordinator) UpdateReplicaSeq(nodeID string, seq uint64) {
	c.replicaSeqs.Store(nodeID, seq)
}
func (c *ClusterCoordinator) ReplicaLag(nodeID string) uint64 {
	v, ok := c.replicaSeqs.Load(nodeID)
	if !ok {
		return 0
	}
	rs := v.(uint64)
	cs := c.repLog.CurrentSeq()
	if cs <= rs {
		return 0
	}
	return cs - rs
}
func (c *ClusterCoordinator) MinReplicatedSeq() uint64 {
	var min uint64
	has := false
	c.replicaSeqs.Range(func(key, value any) bool {
		s := value.(uint64)
		if !has || s < min {
			min = s
			has = true
		}
		return true
	})
	if !has {
		return 0
	}
	return min
}

func (c *ClusterCoordinator) GracefulFailover(ctx context.Context, targetNodeID string) error {
	c.failoverMu.Lock()
	defer c.failoverMu.Unlock()
	if !c.IsLeader() {
		return errors.New("graceful failover: not the Cortex")
	}
	peer, ok := c.mgr.GetPeer(targetNodeID)
	if !ok {
		return fmt.Errorf("graceful failover: target %q is not a known peer", targetNodeID)
	}
	c.nodeState.Store(uint32(StateDraining))
	okHandoff := false
	defer func() {
		if !okHandoff {
			c.nodeState.Store(uint32(StateNormal))
		}
	}()
	if c.hebbianFlusher != nil {
		c.hebbianFlusher.Stop()
	}
	cortexSeq := c.repLog.CurrentSeq()
	convTimeout := time.Duration(c.cfg.FailoverConvergenceTimeoutSec) * time.Second
	if convTimeout <= 0 {
		convTimeout = 30 * time.Second
	}
	convCtx, cancel := context.WithTimeout(ctx, convTimeout)
	defer cancel()
	if err := c.waitForConvergence(convCtx, cortexSeq); err != nil {
		return err
	}
	epoch := c.epochStore.Load()
	payload, err := msgpack.Marshal(HandoffMessage{TargetID: targetNodeID, Epoch: epoch, CortexSeq: cortexSeq})
	if err != nil {
		return err
	}
	c.handoffMu.Lock()
	c.handoffAckCh = make(chan HandoffAck, 1)
	c.handoffMu.Unlock()
	if err := peer.Send(TypeHandoff, payload); err != nil {
		return err
	}
	ackTimeout := time.Duration(c.cfg.HandoffAckTimeoutSec) * time.Second
	if ackTimeout <= 0 {
		ackTimeout = 5 * time.Second
	}
	tm := time.NewTimer(ackTimeout)
	defer tm.Stop()
	defer func() { c.handoffMu.Lock(); c.handoffAckCh = nil; c.handoffMu.Unlock() }()
	select {
	case ack := <-c.handoffAckCh:
		if !ack.Success {
			return errors.New("graceful failover: target rejected handoff")
		}
	case <-tm.C:
		return errors.New("graceful failover: HANDOFF_ACK timeout")
	case <-ctx.Done():
		return ctx.Err()
	}
	okHandoff = true
	c.nodeState.Store(uint32(StateNormal))
	c.handleDemotion()
	return nil
}

func (c *ClusterCoordinator) waitForConvergence(ctx context.Context, targetSeq uint64) error {
	if targetSeq == 0 {
		return nil
	}
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return errors.New("convergence timeout")
		case <-t.C:
			if c.allReplicasConverged(targetSeq) {
				return nil
			}
		}
	}
}

func (c *ClusterCoordinator) allReplicasConverged(targetSeq uint64) bool {
	all := true
	has := false
	c.replicaSeqs.Range(func(key, value any) bool {
		has = true
		if value.(uint64) < targetSeq {
			all = false
			return false
		}
		return true
	})
	if !has {
		return true
	}
	return all
}

func (c *ClusterCoordinator) HandleHandoff(fromNodeID string, payload []byte) error {
	var msg HandoffMessage
	if err := msgpack.Unmarshal(payload, &msg); err != nil {
		return err
	}
	if msg.TargetID != c.cfg.NodeID {
		return fmt.Errorf("handoff target mismatch")
	}
	newEpoch := msg.Epoch + 1
	if err := c.epochStore.ForceSet(newEpoch); err != nil {
		return err
	}
	if actual := c.epochStore.Load(); actual != newEpoch {
		return fmt.Errorf("epoch already advanced to %d", actual)
	}
	if err := c.epochStore.PersistRole("cortex"); err != nil {
		return err
	}
	claimPayload, err := msgpack.Marshal(CortexClaim{CortexID: c.cfg.NodeID, Epoch: newEpoch, FencingToken: newEpoch})
	if err == nil {
		c.mgr.Broadcast(TypeCortexClaim, claimPayload)
	}
	c.election.mu.Lock()
	c.election.state = ElectionLeader
	c.election.currentLeader = c.cfg.NodeID
	c.election.mu.Unlock()
	c.handlePromotion(newEpoch)
	ackPayload, err := msgpack.Marshal(HandoffAck{TargetID: c.cfg.NodeID, Epoch: newEpoch, Success: true})
	if err != nil {
		return err
	}
	peer, ok := c.mgr.GetPeer(fromNodeID)
	if !ok {
		return fmt.Errorf("cannot send ack")
	}
	if err := peer.Send(TypeHandoffAck, ackPayload); err != nil {
		return err
	}
	_ = c.epochStore.PersistRole("")
	return nil
}

func (c *ClusterCoordinator) SetReconciler(rec *Reconciler) {
	if c.started.Load() {
		panic("SetReconciler called after Run()")
	}
	c.reconciler = rec
}
func (c *ClusterCoordinator) SetMOL(mol WALPruner) {
	if c.started.Load() {
		panic("SetMOL called after Run()")
	}
	c.mol = mol
}
func (c *ClusterCoordinator) SetReconcileOnHeal(enabled bool) {
	if enabled {
		c.reconcileOnHeal.Store(1)
	} else {
		c.reconcileOnHeal.Store(0)
	}
}
func (c *ClusterCoordinator) GetClusterConfig() *ClusterConfig { return c.cfg }
func (c *ClusterCoordinator) CCSProbe() *CCSProbe              { return c.ccsProbe }
func (c *ClusterCoordinator) IncrementSnapshotCount()          { c.snapshotInProgress.Add(1) }
func (c *ClusterCoordinator) DecrementSnapshotCount()          { c.snapshotInProgress.Add(-1) }
func (c *ClusterCoordinator) SnapshotInProgress() bool         { return c.snapshotInProgress.Load() > 0 }

func (c *ClusterCoordinator) startPeriodicPrune(ctx context.Context) {
	if c.mol == nil {
		return
	}
	go func() {
		intv := time.Duration(c.cfg.PruneIntervalSec) * time.Second
		if intv <= 0 {
			intv = 60 * time.Second
		}
		t := time.NewTicker(intv)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if !c.IsLeader() || c.snapshotInProgress.Load() > 0 {
					continue
				}
				min := c.MinReplicatedSeq()
				if min == 0 {
					continue
				}
				_ = c.mol.PruneApplied(min)
			}
		}
	}()
}

func (c *ClusterCoordinator) RemoveNode(nodeID string) error {
	if nodeID == c.cfg.NodeID {
		return ErrSelfRemoval
	}
	c.streamersMu.Lock()
	if cancel, ok := c.streamers[nodeID]; ok {
		cancel()
		delete(c.streamers, nodeID)
	}
	c.streamersMu.Unlock()
	c.msp.RemovePeer(nodeID)
	c.replicaSeqs.Delete(nodeID)
	c.election.UnregisterVoter(nodeID)
	c.mgr.RemovePeer(nodeID)
	return nil
}

func (c *ClusterCoordinator) TriggerReconciliation(ctx context.Context, lobeNodeIDs []string) (ReconcileResult, error) {
	if c.reconciler == nil {
		return ReconcileResult{}, errors.New("reconciler not configured")
	}
	return c.reconciler.Run(ctx, lobeNodeIDs)
}

func (c *ClusterCoordinator) HandleHandoffAck(fromNodeID string, payload []byte) error {
	_ = fromNodeID
	var ack HandoffAck
	if err := msgpack.Unmarshal(payload, &ack); err != nil {
		return err
	}
	c.handoffMu.Lock()
	ch := c.handoffAckCh
	c.handoffMu.Unlock()
	if ch != nil {
		select {
		case ch <- ack:
		default:
		}
	}
	return nil
}
