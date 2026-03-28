package replication

import "time"

// ConsistencyMode defines the replication consistency level.
type ConsistencyMode uint8

const (
	ModeEventual         ConsistencyMode = 1
	ModeStrong           ConsistencyMode = 2
	ModeBoundedStaleness ConsistencyMode = 3
)

// WALOp defines the type of operation written to the replication log.
type WALOp uint8

const (
	OpSet       WALOp = 1
	OpDelete    WALOp = 2
	OpBatch     WALOp = 3
	OpCognitive WALOp = 4 // Hebbian/Decay/Confidence state updates
	OpIndex     WALOp = 5 // FTS or HNSW index updates
	OpMeta      WALOp = 6 // cluster metadata (epoch changes, membership)
)

// ReplicationEntry represents a single entry in the replication log.
type ReplicationEntry struct {
	Seq         uint64
	Op          WALOp
	Key         []byte
	Value       []byte
	TimestampNS int64
}

// NodeRole defines the role a node plays in the replication cluster.
type NodeRole uint8

const (
	RoleUnknown  NodeRole = 0 // startup / uninitialized
	RolePrimary  NodeRole = 1 // Cortex — cognitive brain, accepts all writes
	RoleReplica  NodeRole = 2 // Lobe — receives replication, serves reads
	RoleSentinel NodeRole = 3 // quorum voter only, no data storage
	RoleObserver NodeRole = 4 // receives replication, no election vote
)

// NodeInfo describes a node in the replication topology.
type NodeInfo struct {
	NodeID   string
	Addr     string
	Role     NodeRole
	LastSeq  uint64
	LeaseExp time.Time // lease expiration time (for primaries)
}

// String returns the string representation of a NodeRole.
func (r NodeRole) String() string {
	switch r {
	case RolePrimary:
		return "primary"
	case RoleReplica:
		return "replica"
	case RoleSentinel:
		return "sentinel"
	case RoleObserver:
		return "observer"
	default:
		return "unknown"
	}
}

// String returns the string representation of a WALOp.
func (op WALOp) String() string {
	switch op {
	case OpSet:
		return "set"
	case OpDelete:
		return "delete"
	case OpBatch:
		return "batch"
	case OpCognitive:
		return "cognitive"
	case OpIndex:
		return "index"
	case OpMeta:
		return "meta"
	default:
		return "unknown"
	}
}
