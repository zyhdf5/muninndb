package replication

import "time"

type ConsistencyMode uint8

const (
	ModeEventual         ConsistencyMode = 1
	ModeStrong           ConsistencyMode = 2
	ModeBoundedStaleness ConsistencyMode = 3
)

type WALOp uint8

const (
	OpSet       WALOp = 1
	OpDelete    WALOp = 2
	OpBatch     WALOp = 3
	OpCognitive WALOp = 4
	OpIndex     WALOp = 5
	OpMeta      WALOp = 6
)

type ReplicationEntry struct {
	Seq         uint64
	Op          WALOp
	Key         []byte
	Value       []byte
	TimestampNS int64
}

type NodeRole uint8

const (
	RoleUnknown  NodeRole = 0
	RolePrimary  NodeRole = 1
	RoleReplica  NodeRole = 2
	RoleSentinel NodeRole = 3
	RoleObserver NodeRole = 4
)

type NodeInfo struct {
	NodeID   string
	Addr     string
	Role     NodeRole
	LastSeq  uint64
	LeaseExp time.Time
}

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
