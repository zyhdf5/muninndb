package replication

import (
	"encoding/binary"
	"errors"
	"io"
)

type Frame struct {
	Version       uint8
	Type          uint8
	Flags         uint16
	PayloadLength uint32
	CorrelationID uint64
	Payload       []byte
}

const (
	FramePrefixSize = 16
	MaxPayloadSize  = 16 * 1024 * 1024
)

const (
	FlagCompressed uint16 = 1 << 0
	FlagStreaming  uint16 = 1 << 1
	FlagLastFrame  uint16 = 1 << 2
	FlagVault      uint16 = 1 << 3
	FlagUrgent     uint16 = 1 << 4
)

const (
	TypeHello        uint8 = 0x01
	TypeHelloOK      uint8 = 0x02
	TypeWrite        uint8 = 0x03
	TypeWriteOK      uint8 = 0x04
	TypeRead         uint8 = 0x05
	TypeReadResp     uint8 = 0x06
	TypeActivate     uint8 = 0x07
	TypeActivateResp uint8 = 0x08
	TypeSubscribe    uint8 = 0x09
	TypeSubOK        uint8 = 0x0A
	TypeActivation   uint8 = 0x0B
	TypeLink         uint8 = 0x0C
	TypeLinkOK       uint8 = 0x0D
	TypeForget       uint8 = 0x0E
	TypeForgetOK     uint8 = 0x0F
	TypeUnsub        uint8 = 0x10
	TypeUnsubOK      uint8 = 0x11
	TypeStat         uint8 = 0x12
	TypeStatResp     uint8 = 0x13
	TypePing         uint8 = 0x14
	TypePong         uint8 = 0x15
	TypeError        uint8 = 0xFF

	TypeReplEntry uint8 = 0x20
	TypeReplBatch uint8 = 0x21
	TypeReplAck   uint8 = 0x22
	TypeReplNack  uint8 = 0x23

	TypeSnapHeader   uint8 = 0x24
	TypeSnapChunk    uint8 = 0x25
	TypeSnapAck      uint8 = 0x26
	TypeSnapComplete uint8 = 0x27

	TypeCogForward uint8 = 0x28
	TypeCogAck     uint8 = 0x29

	TypeVoteRequest  uint8 = 0x30
	TypeVoteResponse uint8 = 0x31
	TypeCortexClaim  uint8 = 0x32
	TypeSDown        uint8 = 0x33
	TypeODown        uint8 = 0x34
	TypeGossip       uint8 = 0x35
	TypeJoinRequest  uint8 = 0x36
	TypeJoinResponse uint8 = 0x37
	TypeLeave        uint8 = 0x38

	TypeHandoff    uint8 = 0x39
	TypeHandoffAck uint8 = 0x3A

	TypeCCSProbe    uint8 = 0x3B
	TypeCCSResponse uint8 = 0x3C

	TypeReconProbe uint8 = 0x3D
	TypeReconReply uint8 = 0x3E
	TypeReconSync  uint8 = 0x3F
	TypeReconAck   uint8 = 0x40
)

const CurrentProtocolVersion uint16 = 1

var MinSupportedProtocolVersion uint16 = 0
var DeprecatedProtocolVersion uint16 = 0

var (
	ErrPayloadTooLarge = errors.New("payload exceeds 16 MB limit")
	ErrVersionMismatch = errors.New("unsupported frame version")
)

func ReadFrame(r io.Reader) (*Frame, error) {
	f := &Frame{}
	prefix := make([]byte, FramePrefixSize)
	if _, err := io.ReadFull(r, prefix); err != nil {
		return nil, err
	}
	f.Version = prefix[0]
	f.Type = prefix[1]
	f.Flags = binary.BigEndian.Uint16(prefix[2:4])
	f.PayloadLength = binary.BigEndian.Uint32(prefix[4:8])
	f.CorrelationID = binary.BigEndian.Uint64(prefix[8:16])
	if f.Version != 0x01 {
		return nil, ErrVersionMismatch
	}
	if f.PayloadLength > MaxPayloadSize {
		return nil, ErrPayloadTooLarge
	}
	if f.PayloadLength == 0 {
		f.Payload = []byte{}
		return f, nil
	}
	p := make([]byte, f.PayloadLength)
	if _, err := io.ReadFull(r, p); err != nil {
		return nil, err
	}
	f.Payload = p
	return f, nil
}

func WriteFrame(w io.Writer, f *Frame) error {
	if len(f.Payload) > MaxPayloadSize {
		return ErrPayloadTooLarge
	}
	prefix := make([]byte, FramePrefixSize)
	prefix[0] = f.Version
	prefix[1] = f.Type
	binary.BigEndian.PutUint16(prefix[2:4], f.Flags)
	binary.BigEndian.PutUint32(prefix[4:8], uint32(len(f.Payload)))
	binary.BigEndian.PutUint64(prefix[8:16], f.CorrelationID)
	buf := make([]byte, FramePrefixSize+len(f.Payload))
	copy(buf[:FramePrefixSize], prefix)
	copy(buf[FramePrefixSize:], f.Payload)
	_, err := w.Write(buf)
	return err
}

type VoteRequest struct {
	Epoch       uint64 `msgpack:"epoch"`
	CandidateID string `msgpack:"candidate_id"`
	LastSeq     uint64 `msgpack:"last_seq"`
	ConfigEpoch uint64 `msgpack:"config_epoch"`
}

type VoteResponse struct {
	Epoch   uint64 `msgpack:"epoch"`
	VoterID string `msgpack:"voter_id"`
	Granted bool   `msgpack:"granted"`
}

type CortexClaim struct {
	Epoch        uint64 `msgpack:"epoch"`
	FencingToken uint64 `msgpack:"fencing_token"`
	CortexID     string `msgpack:"cortex_id"`
	CortexAddr   string `msgpack:"cortex_addr"`
}

type ReplEntry struct {
	Seq         uint64 `msgpack:"seq"`
	Op          uint8  `msgpack:"op"`
	Key         []byte `msgpack:"key"`
	Value       []byte `msgpack:"value"`
	TimestampNS int64  `msgpack:"ts"`
}

type ReplBatch struct {
	Entries []ReplEntry `msgpack:"entries"`
}

type ReplAck struct {
	LastSeq uint64 `msgpack:"last_seq"`
	NodeID  string `msgpack:"node_id"`
}

type ReplNack struct {
	FromSeq uint64 `msgpack:"from_seq"`
	NodeID  string `msgpack:"node_id"`
}

type GossipMember struct {
	ID       string `msgpack:"id"`
	Addr     string `msgpack:"addr"`
	Role     uint8  `msgpack:"role"`
	LastSeq  uint64 `msgpack:"last_seq"`
	LastSeen int64  `msgpack:"last_seen"`
}

type GossipMessage struct {
	SenderID string         `msgpack:"sender_id"`
	Epoch    uint64         `msgpack:"epoch"`
	Members  []GossipMember `msgpack:"members"`
}

type JoinRequest struct {
	NodeID          string   `msgpack:"node_id"`
	Addr            string   `msgpack:"addr"`
	LastApplied     uint64   `msgpack:"last_applied"`
	Capabilities    []string `msgpack:"capabilities"`
	SecretHash      []byte   `msgpack:"secret_hash"`
	ProtocolVersion uint16   `msgpack:"proto_ver,omitempty"`
}

type JoinResponse struct {
	Accepted               bool           `msgpack:"accepted"`
	CortexID               string         `msgpack:"cortex_id"`
	CortexAddr             string         `msgpack:"cortex_addr"`
	Epoch                  uint64         `msgpack:"epoch"`
	Members                []GossipMember `msgpack:"members"`
	RejectReason           string         `msgpack:"reject_reason,omitempty"`
	NeedsSnapshot          bool           `msgpack:"needs_snapshot,omitempty"`
	SnapshotSeq            uint64         `msgpack:"snapshot_seq,omitempty"`
	MinProtocolVersion     uint16         `msgpack:"min_proto_ver,omitempty"`
	CurrentProtocolVersion uint16         `msgpack:"current_proto_ver,omitempty"`
}

type SDownNotification struct {
	SenderID  string `msgpack:"sender_id"`
	TargetID  string `msgpack:"target_id"`
	Epoch     uint64 `msgpack:"epoch"`
	Timestamp int64  `msgpack:"timestamp"`
}

type LeaveMessage struct {
	NodeID string `msgpack:"node_id"`
	Epoch  uint64 `msgpack:"epoch"`
}

type SnapHeader struct {
	SnapshotSeq uint64 `msgpack:"snapshot_seq"`
	NodeID      string `msgpack:"node_id"`
	TotalKeys   uint64 `msgpack:"total_keys"`
	Timestamp   int64  `msgpack:"timestamp"`
}

type SnapChunk struct {
	ChunkNum  uint32   `msgpack:"chunk_num"`
	LastChunk bool     `msgpack:"last_chunk"`
	Pairs     []KVPair `msgpack:"pairs"`
}

type KVPair struct {
	Key   []byte `msgpack:"key"`
	Value []byte `msgpack:"value"`
}

type EdgeRef struct {
	Src [16]byte `msgpack:"src"`
	Dst [16]byte `msgpack:"dst"`
}

type CognitiveSideEffect struct {
	QueryID       string            `msgpack:"query_id"`
	OriginNodeID  string            `msgpack:"origin_node_id"`
	Timestamp     int64             `msgpack:"timestamp"`
	CoActivations []CoActivationRef `msgpack:"co_activations,omitempty"`
	AccessedIDs   [][16]byte        `msgpack:"accessed_ids,omitempty"`
	ArchivedEdges []EdgeRef         `msgpack:"archived_edges,omitempty"`
	RestoredEdges []EdgeRef         `msgpack:"restored_edges,omitempty"`
}

type CoActivationRef struct {
	ID    [16]byte `msgpack:"id"`
	Score float64  `msgpack:"score"`
}

type CogAck struct {
	QueryID string `msgpack:"query_id"`
}

type HandoffMessage struct {
	TargetID  string `msgpack:"tid"`
	Epoch     uint64 `msgpack:"epoch"`
	CortexSeq uint64 `msgpack:"seq"`
}

type HandoffAck struct {
	TargetID string `msgpack:"tid"`
	Epoch    uint64 `msgpack:"epoch"`
	Success  bool   `msgpack:"ok"`
}

type ReconProbeMsg struct {
	RequestID string     `msgpack:"rid"`
	Keys      [][16]byte `msgpack:"keys"`
	Weights   []float64  `msgpack:"weights"`
}

type ReconReplyMsg struct {
	RequestID string    `msgpack:"rid"`
	NodeID    string    `msgpack:"node_id"`
	Weights   []float64 `msgpack:"weights"`
}

type ReconSyncMsg struct {
	RequestID string     `msgpack:"rid"`
	Keys      [][16]byte `msgpack:"keys"`
	Weights   []float64  `msgpack:"weights"`
}

type ReconAckMsg struct {
	RequestID string `msgpack:"rid"`
	NodeID    string `msgpack:"node_id"`
	Applied   int    `msgpack:"applied"`
}

type CCSProbeMsg struct {
	SampledKeys [][16]byte `msgpack:"keys"`
	RequestID   string     `msgpack:"rid"`
}

type CCSResponseMsg struct {
	RequestID string `msgpack:"rid"`
	NodeID    string `msgpack:"node_id"`
	Hash      []byte `msgpack:"hash"`
	KeyCount  int    `msgpack:"key_count"`
}
