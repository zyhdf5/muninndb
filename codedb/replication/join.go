package replication

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"net"
	"sync"

	"github.com/vmihailenco/msgpack/v5"
)

type JoinHandler struct {
	localNodeID   string
	clusterSecret string
	epochStore    *EpochStore
	repLog        *ReplicationLog
	db            KVStore
	mgr           *ConnManager
	members       map[string]NodeInfo
	mu            sync.RWMutex
	OnLobeJoined  func(info NodeInfo)
	OnLobeLeft    func(nodeID string)
}

func NewJoinHandler(localNodeID, clusterSecret string, epochStore *EpochStore, repLog *ReplicationLog, mgr *ConnManager) *JoinHandler {
	return &JoinHandler{localNodeID: localNodeID, clusterSecret: clusterSecret, epochStore: epochStore, repLog: repLog, mgr: mgr, members: map[string]NodeInfo{}}
}
func NewJoinHandlerWithDB(localNodeID, clusterSecret string, epochStore *EpochStore, repLog *ReplicationLog, db KVStore, mgr *ConnManager) *JoinHandler {
	h := NewJoinHandler(localNodeID, clusterSecret, epochStore, repLog, mgr)
	h.db = db
	return h
}

func (h *JoinHandler) HandleJoinRequest(req JoinRequest, conn *PeerConn) JoinResponse {
	currentEpoch := h.epochStore.Load()
	if req.NodeID == "" {
		return JoinResponse{Accepted: false, RejectReason: "empty node ID", Epoch: currentEpoch, CortexID: h.localNodeID}
	}
	if h.clusterSecret != "" {
		expectedHash := hmac.New(sha256.New, []byte(h.clusterSecret))
		expectedHash.Write([]byte(req.NodeID))
		if !hmac.Equal(req.SecretHash, expectedHash.Sum(nil)) {
			return JoinResponse{Accepted: false, RejectReason: "invalid cluster secret", Epoch: currentEpoch, CortexID: h.localNodeID}
		}
	}
	if req.ProtocolVersion > CurrentProtocolVersion {
		return JoinResponse{Accepted: false, RejectReason: "protocol too new", MinProtocolVersion: MinSupportedProtocolVersion, CurrentProtocolVersion: CurrentProtocolVersion}
	}
	if req.ProtocolVersion < MinSupportedProtocolVersion {
		return JoinResponse{Accepted: false, RejectReason: "protocol too old", MinProtocolVersion: MinSupportedProtocolVersion, CurrentProtocolVersion: CurrentProtocolVersion}
	}
	if currentEpoch == 0 {
		return JoinResponse{Accepted: false, RejectReason: "cluster not yet bootstrapped (epoch 0)", Epoch: currentEpoch, CortexID: h.localNodeID}
	}
	info := NodeInfo{NodeID: req.NodeID, Addr: req.Addr, Role: RoleReplica, LastSeq: req.LastApplied}
	h.mu.Lock()
	h.members[req.NodeID] = info
	cb := h.OnLobeJoined
	h.mu.Unlock()
	h.mgr.AddPeer(req.NodeID, req.Addr)
	if cb != nil {
		cb(info)
	}
	resp := JoinResponse{Accepted: true, CortexID: h.localNodeID, Epoch: currentEpoch}
	if h.db != nil {
		resp.NeedsSnapshot = true
		resp.SnapshotSeq = h.repLog.CurrentSeq()
	}
	_ = conn
	return resp
}

func (h *JoinHandler) StreamSnapshot(ctx context.Context, conn *PeerConn) (uint64, error) {
	if h.db == nil {
		return 0, fmt.Errorf("join handler: snapshot streaming not configured (no db)")
	}
	return NewSnapshotSender(h.db, h.repLog).Send(ctx, conn)
}

func (h *JoinHandler) HandleLeave(msg LeaveMessage) {
	h.mu.Lock()
	_, exists := h.members[msg.NodeID]
	if exists {
		delete(h.members, msg.NodeID)
	}
	cb := h.OnLobeLeft
	h.mu.Unlock()
	if exists {
		h.mgr.RemovePeer(msg.NodeID)
	}
	if exists && cb != nil {
		cb(msg.NodeID)
	}
}

func (h *JoinHandler) Members() []NodeInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]NodeInfo, 0, len(h.members))
	for _, info := range h.members {
		out = append(out, info)
	}
	return out
}

type JoinResult struct {
	JoinResponse
	StreamFromSeq uint64
}

type JoinClient struct {
	localNodeID   string
	localAddr     string
	clusterSecret string
	epochStore    *EpochStore
	applier       *Applier
	db            KVStore
	mgr           *ConnManager
}

func NewJoinClient(localNodeID, localAddr, clusterSecret string, epochStore *EpochStore, applier *Applier, mgr *ConnManager) *JoinClient {
	return &JoinClient{localNodeID: localNodeID, localAddr: localAddr, clusterSecret: clusterSecret, epochStore: epochStore, applier: applier, mgr: mgr}
}
func NewJoinClientWithDB(localNodeID, localAddr, clusterSecret string, epochStore *EpochStore, applier *Applier, db KVStore, mgr *ConnManager) *JoinClient {
	c := NewJoinClient(localNodeID, localAddr, clusterSecret, epochStore, applier, mgr)
	c.db = db
	return c
}

func (c *JoinClient) Join(ctx context.Context, cortexAddr string) (JoinResult, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", cortexAddr)
	if err != nil {
		return JoinResult{}, err
	}
	defer conn.Close()
	return c.joinConn(ctx, conn)
}

func (c *JoinClient) joinConn(ctx context.Context, conn net.Conn) (JoinResult, error) {
	var lastApplied uint64
	if c.applier != nil {
		lastApplied = c.applier.LastApplied()
	}
	var secretHash []byte
	if c.clusterSecret != "" {
		h := hmac.New(sha256.New, []byte(c.clusterSecret))
		h.Write([]byte(c.localNodeID))
		secretHash = h.Sum(nil)
	}
	req := JoinRequest{NodeID: c.localNodeID, Addr: c.localAddr, LastApplied: lastApplied, SecretHash: secretHash, ProtocolVersion: CurrentProtocolVersion}
	payload, err := msgpack.Marshal(req)
	if err != nil {
		return JoinResult{}, err
	}
	if err := WriteFrame(conn, &Frame{Version: 0x01, Type: TypeJoinRequest, Payload: payload, PayloadLength: uint32(len(payload))}); err != nil {
		return JoinResult{}, err
	}
	respFrame, err := ReadFrame(conn)
	if err != nil {
		return JoinResult{}, err
	}
	if respFrame.Type != TypeJoinResponse {
		return JoinResult{}, fmt.Errorf("join: unexpected frame")
	}
	var resp JoinResponse
	if err := msgpack.Unmarshal(respFrame.Payload, &resp); err != nil {
		return JoinResult{}, err
	}
	if !resp.Accepted {
		return JoinResult{JoinResponse: resp}, fmt.Errorf("join rejected: %s", resp.RejectReason)
	}
	if err := c.epochStore.ForceSet(resp.Epoch); err != nil {
		return JoinResult{JoinResponse: resp}, err
	}
	if resp.CortexID != "" && resp.CortexAddr != "" {
		c.mgr.AddPeer(resp.CortexID, resp.CortexAddr)
	}
	res := JoinResult{JoinResponse: resp, StreamFromSeq: lastApplied}
	if resp.NeedsSnapshot {
		snapSeq, err := c.receiveSnapshot(ctx, conn)
		if err != nil {
			return res, err
		}
		res.StreamFromSeq = snapSeq
	}
	return res, nil
}

func (c *JoinClient) receiveSnapshot(ctx context.Context, conn net.Conn) (uint64, error) {
	if c.db == nil {
		return 0, fmt.Errorf("join: snapshot reception requires a DB")
	}
	return NewSnapshotReceiver(c.db).Receive(ctx, &PeerConn{conn: conn})
}

func (c *JoinClient) Leave(ctx context.Context, cortexAddr string) error {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", cortexAddr)
	if err != nil {
		return err
	}
	defer conn.Close()
	return c.leaveConn(conn)
}

func (c *JoinClient) leaveConn(conn net.Conn) error {
	payload, err := msgpack.Marshal(LeaveMessage{NodeID: c.localNodeID, Epoch: c.epochStore.Load()})
	if err != nil {
		return err
	}
	return WriteFrame(conn, &Frame{Version: 0x01, Type: TypeLeave, PayloadLength: uint32(len(payload)), Payload: payload})
}
