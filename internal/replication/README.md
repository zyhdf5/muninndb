# MuninnDB Replication Package

WAL streaming replication with automatic failover using lease-based leader election and fencing tokens.

## Quick Start

### Initialize

```go
import "github.com/scrypster/muninndb/internal/replication"

// Create replication log (primary + replicas)
repLog := replication.NewReplicationLog(db)

// Create applier (replicas only)
applier := replication.NewApplier(db)

// Create lease backend (use Memory for testing, etcd for production)
backend := replication.NewMemoryLeaseBackend()

// Create leader elector
elector := replication.NewLeaderElector("node-id", backend)

// Create coordinator
coord := replication.NewCoordinator("node-id", repLog, applier, elector)

// Start election loop
go elector.Run(ctx)
```

### Log Writes

```go
// After batch commit, async append to replication log
go func() {
    _, _ = repLog.Append(replication.OpSet, key, value)
}()
```

### Replicate to Peers

```go
// On primary, stream to each replica
streamer := replication.NewStreamer(repLog)
go streamer.Stream(ctx, lastAckSeq)

for entry := range streamer.Entries() {
    // Send to replica via gRPC
    replica.ApplyEntry(entry)
}
```

### Apply on Replica

```go
// When replica receives entry
if err := applier.Apply(entry); err != nil {
    return err
}
```

## Files

| File | Purpose |
|------|---------|
| `types.go` | Type definitions (ConsistencyMode, WALOp, NodeRole, etc.) |
| `log.go` | Append-only replication log backed by Pebble |
| `applier.go` | Apply entries to local Pebble (replica-side) |
| `streamer.go` | Stream entries from log (primary-side) |
| `fence.go` | Fencing token validation (split-brain prevention) |
| `lease.go` | LeaseBackend interface (abstraction for etcd/Consul) |
| `memory_backend.go` | In-memory LeaseBackend implementation |
| `leader.go` | Lease-based leader election |
| `coordinator.go` | High-level replication state management |
| `log_test.go` | Comprehensive unit tests |
| `DESIGN.md` | Design document and decision rationale |
| `INTEGRATION.md` | Integration guide for application code |

## Architecture

```
┌─────────────────────────────────────────┐
│        Application (MuninnDB)           │
└─────────────────────────────────────────┘
                    ↓
┌─────────────────────────────────────────┐
│         PebbleStore (Storage)           │
│  ┌───────────────────────────────────┐  │
│  │    Pebble DB                      │  │
│  │    0x01-0x17: Application keys    │  │
│  │    0x19: Replication log entries  │  │
│  └───────────────────────────────────┘  │
└─────────────────────────────────────────┘
                    ↓
     ┌──────────────┴──────────────┐
     ↓                             ↓
┌──────────────────┐     ┌──────────────────┐
│   ReplicationLog │     │  LeaderElector   │
│  (All nodes)     │     │  (All nodes)     │
│                  │     │                  │
│ • Append entries │     │ • Try acquire    │
│ • Read entries   │     │ • Renew lease    │
│ • Prune old      │     │ • Fencing token  │
└──────────────────┘     └──────────────────┘
                              ↓ IsLeader
                        ┌─────────────┐
                        │   Primary?  │
                        └─────────────┘
                         Yes↓      ↓No
                    ┌──────┐    ┌──────────┐
                    ↓      ↓    ↓          ↓
              ┌─────────┐  ┌─────────┐  ┌──────────┐
              │Streamer │  │Applier  │  │Streamer  │
              │(Primary)│  │(Replica)│  │(nothing) │
              └─────────┘  └─────────┘  └──────────┘
                    ↓           ↓
              [gRPC Stream] [Apply locally]
```

## Key Prefix

**Replication uses Pebble key prefix `0x19`:**

```
Replication log entries:
  Key: 0x19 | seq_be64(8) = 9 bytes
  Value: msgpack-encoded ReplicationEntry

Sequence counter:
  Key: 0x19 | 0xFF * 8 = 9 bytes
  Value: counter as uint64 big-endian
```

## Consistency Modes

| Mode | Behavior | Data Loss | Latency |
|------|----------|-----------|---------|
| **Eventual** | Async replication | Possible | Low |
| **Strong** | Wait for all replicas | None | High |
| **BoundedStaleness** | Wait for lag < N | Partial | Medium |

## Thread Safety

All components are thread-safe:
- **ReplicationLog**: Mutex protects sequence counter; Pebble handles concurrent entry writes
- **Applier**: Mutex protects lastApplied; Pebble handles concurrent reads
- **LeaderElector**: Atomic fields for IsLeader/Token
- **Coordinator**: Mutex protects replica map

## Tests

13 comprehensive unit tests covering:

```bash
go test ./internal/replication/... -v

TestReplicationLog_AppendAndRead      ✓  Basic append/read
TestReplicationLog_ReadSince          ✓  Range queries
TestReplicationLog_ReadSince_WithLimit ✓  Pagination
TestReplicationLog_Prune              ✓  Cleanup
TestReplicationLog_CurrentSeq         ✓  Sequence tracking
TestReplicationLog_Persistence        ✓  Restarts
TestApplier_Apply                     ✓  Idempotent application
TestApplier_IsLagging                 ✓  Lag detection
TestFencingToken_Valid                ✓  Token validation
TestFencingToken_StaleRejected        ✓  Split-brain prevention
TestLeaderElector_BasicElection       ✓  Lease acquisition
TestLeaderElector_FencingToken        ✓  Token increment
```

**All tests passing:** ✓

## REST Endpoints

```
GET  /v1/replication/status
POST /v1/replication/promote
GET  /v1/replication/lag
```

See `replication_handlers.go` for details.

## Production Checklist

- [ ] Implement etcd/Consul LeaseBackend
- [ ] Set up monitoring for lag/token
- [ ] Configure log pruning strategy
- [ ] Test multi-node failover
- [ ] Document operational procedures
- [ ] Set up alerting for replication lag
- [ ] Plan state transfer for new replicas
- [ ] Document consistency guarantees for app

## Limitations

- MemoryLeaseBackend not suitable for production
- No automatic replica discovery
- No state transfer protocol (replicas must start empty)
- Log grows unbounded until manually pruned
- No compression or compaction

## See Also

- [DESIGN.md](DESIGN.md) - Design decisions and architecture
- [INTEGRATION.md](INTEGRATION.md) - Integration guide for applications

## Example: Two-Node Setup

```go
// Node 1 (Primary)
node1Backend := replication.NewMemoryLeaseBackend()
node1Elector := replication.NewLeaderElector("node1", node1Backend)
node1Coord := replication.NewCoordinator("node1", repLog, applier, node1Elector)

// Node 2 (Replica)
node2Backend := node1Backend  // Shared for testing
node2Elector := replication.NewLeaderElector("node2", node2Backend)
node2Coord := replication.NewCoordinator("node2", repLog2, applier2, node2Elector)

// Start both
go node1Elector.Run(ctx)
go node2Elector.Run(ctx)

// Node 1 becomes primary (acquires lease first)
node1Coord.Role() // → RolePrimary

// Node 2 is replica
node2Coord.Role() // → RoleReplica

// Write on primary
seq, _ := node1Coord.log.Append(OpSet, key, value)

// Replica streams and applies
entries, _ := node1Coord.log.ReadSince(0, 100)
for _, e := range entries {
    node2Coord.applier.Apply(e)
}
```

## License

Part of MuninnDB. See repository LICENSE.
