package replication

import (
	"context"
	"errors"
	"sync"

	"github.com/vmihailenco/msgpack/v5"
)

type ElectionState uint8

const (
	ElectionIdle ElectionState = iota
	ElectionCandidate
	ElectionLeader
	ElectionFollower
)

type Election struct {
	localNodeID    string
	epochStore     *EpochStore
	mgr            *ConnManager
	isSentinel     bool
	isObserver     bool
	state          ElectionState
	currentLeader  string
	votes          map[uint64]map[string]bool
	votedFor       map[uint64]string
	candidateEpoch uint64
	mu             sync.Mutex
	OnPromoted     func(epoch uint64)
	OnDemoted      func()
	OnNewLeader    func(leaderID string, epoch uint64)
	voters         map[string]struct{}
}

var errAlreadyCandidate = errors.New("election: already a candidate or leader")
var errEpochCASFailed = errors.New("election: epoch compare-and-set failed (concurrent election)")
var errSentinelCannotElect = errors.New("election: sentinel nodes cannot initiate elections")
var errObserverCannotElect = errors.New("election: observer nodes cannot initiate elections")

func NewElection(localNodeID string, epochStore *EpochStore, mgr *ConnManager) *Election {
	return &Election{localNodeID: localNodeID, epochStore: epochStore, mgr: mgr, state: ElectionIdle, votes: map[uint64]map[string]bool{}, votedFor: map[uint64]string{}, voters: map[string]struct{}{}}
}

func (e *Election) SetSentinel(sentinel bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.isSentinel = sentinel
}
func (e *Election) SetObserver(observer bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.isObserver = observer
}
func (e *Election) RegisterVoter(nodeID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.voters[nodeID] = struct{}{}
}
func (e *Election) UnregisterVoter(nodeID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.voters, nodeID)
}

func (e *Election) StartElection(ctx context.Context) error {
	e.mu.Lock()
	if e.isSentinel {
		e.mu.Unlock()
		return errSentinelCannotElect
	}
	if e.isObserver {
		e.mu.Unlock()
		return errObserverCannotElect
	}
	if e.state == ElectionCandidate || e.state == ElectionLeader {
		e.mu.Unlock()
		return errAlreadyCandidate
	}
	currentEpoch := e.epochStore.Load()
	newEpoch := currentEpoch + 1
	ok, err := e.epochStore.CompareAndSet(currentEpoch, newEpoch)
	if err != nil {
		e.mu.Unlock()
		return err
	}
	if !ok {
		e.mu.Unlock()
		return errEpochCASFailed
	}
	e.state = ElectionCandidate
	e.candidateEpoch = newEpoch
	e.votedFor[newEpoch] = e.localNodeID
	if e.votes[newEpoch] == nil {
		e.votes[newEpoch] = map[string]bool{}
	}
	e.votes[newEpoch][e.localNodeID] = true
	quorum := e.quorumLocked()
	voteCount := len(e.votes[newEpoch])
	e.mu.Unlock()
	req := VoteRequest{CandidateID: e.localNodeID, Epoch: newEpoch}
	payload, err := msgpack.Marshal(req)
	if err != nil {
		return err
	}
	e.mgr.Broadcast(TypeVoteRequest, payload)
	if voteCount >= quorum {
		e.tryPromote(newEpoch)
	}
	return nil
}

func (e *Election) HandleVoteRequest(req VoteRequest) VoteResponse {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.isObserver {
		return VoteResponse{VoterID: e.localNodeID, Epoch: req.Epoch, Granted: false}
	}
	currentEpoch := e.epochStore.Load()
	if req.Epoch < currentEpoch {
		return VoteResponse{VoterID: e.localNodeID, Epoch: req.Epoch, Granted: false}
	}
	if req.Epoch > currentEpoch {
		_ = e.epochStore.ForceSet(req.Epoch)
	}
	if prev, ok := e.votedFor[req.Epoch]; ok {
		return VoteResponse{VoterID: e.localNodeID, Epoch: req.Epoch, Granted: prev == req.CandidateID}
	}
	e.votedFor[req.Epoch] = req.CandidateID
	if e.state == ElectionCandidate && e.candidateEpoch < req.Epoch {
		e.state = ElectionIdle
	}
	return VoteResponse{VoterID: e.localNodeID, Epoch: req.Epoch, Granted: true}
}

func (e *Election) HandleVoteResponse(resp VoteResponse) {
	e.mu.Lock()
	if e.state != ElectionCandidate || resp.Epoch != e.candidateEpoch || !resp.Granted {
		e.mu.Unlock()
		return
	}
	if e.votes[resp.Epoch] == nil {
		e.votes[resp.Epoch] = map[string]bool{}
	}
	e.votes[resp.Epoch][resp.VoterID] = true
	quorum := e.quorumLocked()
	voteCount := len(e.votes[resp.Epoch])
	e.mu.Unlock()
	if voteCount >= quorum {
		e.tryPromote(resp.Epoch)
	}
}

func (e *Election) tryPromote(epoch uint64) {
	e.mu.Lock()
	if e.isSentinel {
		e.mu.Unlock()
		return
	}
	if e.state != ElectionCandidate || e.candidateEpoch != epoch {
		e.mu.Unlock()
		return
	}
	ok, err := e.epochStore.CompareAndSet(epoch, epoch)
	if err != nil || !ok {
		e.state = ElectionIdle
		e.mu.Unlock()
		return
	}
	e.state = ElectionLeader
	e.currentLeader = e.localNodeID
	onPromoted := e.OnPromoted
	for ep := range e.votes {
		if ep < epoch {
			delete(e.votes, ep)
		}
	}
	for ep := range e.votedFor {
		if ep < epoch {
			delete(e.votedFor, ep)
		}
	}
	e.mu.Unlock()
	claim := CortexClaim{CortexID: e.localNodeID, Epoch: epoch, FencingToken: epoch}
	payload, err := msgpack.Marshal(claim)
	if err == nil {
		e.mgr.Broadcast(TypeCortexClaim, payload)
	}
	if onPromoted != nil {
		onPromoted(epoch)
	}
}

func (e *Election) HandleCortexClaim(claim CortexClaim) {
	e.mu.Lock()
	currentEpoch := e.epochStore.Load()
	if claim.Epoch < currentEpoch {
		e.mu.Unlock()
		return
	}
	_ = e.epochStore.ForceSet(claim.Epoch)
	wasLeader := e.state == ElectionLeader
	e.state = ElectionFollower
	e.currentLeader = claim.CortexID
	onDemoted, onNewLeader := e.OnDemoted, e.OnNewLeader
	for ep := range e.votes {
		if ep < claim.Epoch {
			delete(e.votes, ep)
		}
	}
	for ep := range e.votedFor {
		if ep < claim.Epoch {
			delete(e.votedFor, ep)
		}
	}
	e.mu.Unlock()
	if wasLeader && onDemoted != nil {
		onDemoted()
	}
	if onNewLeader != nil {
		onNewLeader(claim.CortexID, claim.Epoch)
	}
}

func (e *Election) State() ElectionState  { e.mu.Lock(); defer e.mu.Unlock(); return e.state }
func (e *Election) CurrentLeader() string { e.mu.Lock(); defer e.mu.Unlock(); return e.currentLeader }
func (e *Election) CurrentEpoch() uint64  { return e.epochStore.Load() }
func (e *Election) Quorum() int           { e.mu.Lock(); defer e.mu.Unlock(); return e.quorumLocked() }
func (e *Election) quorumLocked() int     { return len(e.voters)/2 + 1 }
