# WAL Streaming Replication Integration Guide

## Overview

The replication package implements WAL streaming replication with automatic failover for MuninnDB. The system uses lease-based leader election with fencing tokens to prevent split-brain writes.

## Key Prefix

The replication log uses Pebble key prefix `0x19` (25 in decimal), which is available in the MuninnDB key space:

- **Replication log entries**: `0x19 | seq_be64(8)` = 9 bytes
- **Sequence counter**: `0x19 | 0xFF | 0xFF | 0xFF | 0xFF | 0xFF | 0xFF | 0xFF | 0xFF` = 9 bytes

Entries are serialized with msgpack for efficient storage and retrieval.

## Architecture

### Core Components

1. **ReplicationLog** (`log.go`)
   - Append-only log backed by Pebble
   - Stores entries with monotonically increasing sequence numbers
   - Thread-safe with lock-based synchronization
   - Supports pruning old entries once all replicas acknowledge them

2. **Applier** (`applier.go`)
   - Applies replication entries to local Pebble database
   - Tracks LastApplied sequence number
   - Idempotent: skips already-applied entries
   - Supports BoundedStaleness consistency checking

3. **Streamer** (`streamer.go`)
   - Polls replication log for new entries
   - Streams entries to replicas over channels
   - Configurable poll interval (default 100ms)
   - Clean shutdown support

4. **LeaseBackend Interface** (`lease.go`)
   - Abstraction for distributed lease management
   - Supports etcd, Consul, or other consensus systems
   - Provides fencing tokens for split-brain protection

5. **MemoryLeaseBackend** (`memory_backend.go`)
   - In-memory implementation for testing and single-node deployments
   - **Not suitable for production multi-node clusters**

6. **LeaderElector** (`leader.go`)
   - Implements lease-based leader election
   - Automatic renewal with configurable TTL and interval
   - Callbacks for promotion/demotion events
   - Maintains monotonically increasing fencing token

7. **Coordinator** (`coordinator.go`)
   - High-level replication state management
   - Tracks known replicas and their ack'd sequences
   - Computes safe pruning thresholds for different consistency modes
   - Configurable replication mode (Eventual, Strong, BoundedStaleness)

## Integration Steps

### 1. Initialize Replication Components

```go
import "github.com/scrypster/muninndb/internal/replication"

// In your server initialization:
db, _ := pebble.Open(dbPath, opts)

// Create replication log
repLog := replication.NewReplicationLog(db)

// Create applier for replicas (skip for primary-only nodes)
applier := replication.NewApplier(db)

// Create lease backend (use memory for testing, etcd for production)
backend := replication.NewMemoryLeaseBackend()

// Create leader elector
elector := replication.NewLeaderElector("node-id", backend)

// Create coordinator
coord := replication.NewCoordinator("node-id", repLog, applier, elector)
coord.SetReplicationMode(replication.ModeStrong)

// Start leader election in a goroutine
go func() {
    if err := elector.Run(ctx); err != nil {
        log.Printf("leader elector failed: %v", err)
    }
}()
```

### 2. Log Replication Entries

After each successful Pebble batch commit in your write path, append to the replication log:

```go
// In PebbleStore.WriteEngram() or similar:
batch := db.NewBatch()
defer batch.Close()

// ... your write operations ...

if err := batch.Commit(nil); err != nil {
    return err
}

// Async append to replication log (non-blocking)
go func() {
    _, _ = repLog.Append(replication.OpSet, key, value)
}()
```

**Important**: Never block primary writes waiting for replication log. Use a goroutine.

### 3. Stream to Replicas

```go
// On the primary, for each connected replica:
streamer := replication.NewStreamer(repLog)

lastAckSeq := uint64(0) // from replica state

go func() {
    ctx := context.Background()
    if err := streamer.Stream(ctx, lastAckSeq); err != nil {
        log.Printf("streamer failed: %v", err)
    }
}()

// Read from streamer.Entries() and send to replica via gRPC
for entry := range streamer.Entries() {
    // Send entry to replica over gRPC
    resp, err := replicaClient.ApplyEntry(ctx, &pb.ApplyEntryRequest{
        Seq:   entry.Seq,
        Op:    int32(entry.Op),
        Key:   entry.Key,
        Value: entry.Value,
    })
    if err == nil && resp.Success {
        coord.UpdateReplicaSeq(replicaID, entry.Seq)
    }
}
```

### 4. Apply Entries on Replicas

```go
// When replica receives entry from primary:
entry := replication.ReplicationEntry{
    Seq:   req.Seq,
    Op:    replication.WALOp(req.Op),
    Key:   req.Key,
    Value: req.Value,
}

if err := applier.Apply(entry); err != nil {
    log.Printf("apply failed: %v", err)
    return false
}

return true
```

### 5. REST Endpoints

The REST server provides three new endpoints:

```
GET  /v1/replication/status    → ReplicationStatusResponse
GET  /v1/replication/lag       → ReplicationLagResponse
POST /v1/replication/promote   → PromoteReplicaResponse
```

Example:
```bash
# Check replication status
curl http://localhost:8888/v1/replication/status

# Promote replica to primary (emergency)
curl -X POST http://localhost:8888/v1/replication/promote \
  -H "Content-Type: application/json" \
  -d '{"force": true}'
```

## Consistency Modes

### ModeEventual (Default)
- Writes return immediately after primary commit
- No waiting for replica acknowledgment
- Replicas are best-effort
- Lowest latency, highest throughput
- Acceptable data loss in failover

### ModeStrong
- Writes wait for all known replicas to acknowledge
- Guarantees no data loss
- Higher latency
- Better for critical data

### ModeBoundedStaleness
- Writes wait until replication lag is within bounds
- Replicas can be at most N entries behind
- Compromise between latency and durability
- Useful for consistency-critical but latency-sensitive applications

## Split-Brain Protection

The system uses fencing tokens to prevent split-brain writes:

1. Every time the lease changes hands, the token increments
2. Each write request from a primary includes its current fencing token
3. Replicas validate incoming tokens with `ValidateFencingToken(currentToken, providedToken)`
4. If `providedToken < currentToken`, the request is rejected with `ErrStaleFencingToken`

This prevents a demoted primary from writing data that would conflict with the new primary.

## Production Setup: Etcd Backend

To use etcd instead of the in-memory backend:

```go
// Create etcd client
cli, _ := clientv3.New(clientv3.Config{
    Endpoints: []string{"localhost:2379"},
})

// Implement LeaseBackend using etcd
type EtcdLeaseBackend struct {
    cli *clientv3.Client
    leaseID clientv3.LeaseID
    token uint64
}

func (e *EtcdLeaseBackend) TryAcquire(ctx context.Context, nodeID string, ttl time.Duration) (bool, error) {
    // Use etcd session for lease acquisition
    // Grant lease, store lock at /muninndb/leader with nodeID
    // Return true if acquired, false if held by another
}

// Similar implementations for Renew, Release, CurrentHolder, Token

// Then use it:
backend := &EtcdLeaseBackend{cli: cli}
elector := replication.NewLeaderElector("node-id", backend)
```

## Log Pruning Strategy

To prevent unbounded log growth:

```go
// Periodically compute safe pruning threshold
go func() {
    ticker := time.NewTicker(30 * time.Second)
    for range ticker.C {
        threshold := coord.ComputeAckThreshold(coord.ReplicationMode())

        // Keep 1000 entries for safety margin
        if threshold > 1000 {
            if err := repLog.Prune(threshold - 1000); err != nil {
                log.Printf("prune failed: %v", err)
            }
        }
    }
}()
```

## Testing

Comprehensive tests are included in `log_test.go`:

```bash
# Run all tests
go test ./internal/replication/... -v

# Test specific scenarios
go test -run TestReplicationLog_Persistence -v
go test -run TestLeaderElector_FencingToken -v
```

Key test scenarios:
- Append and read entries
- Sequence number persistence across restarts
- Replication entry application
- Fencing token validation
- Leader election with multiple nodes
- Lag detection for BoundedStaleness mode

## Configuration Recommendations

| Setting | Value | Notes |
|---------|-------|-------|
| Lease TTL | 10s | Time a node holds leader lease |
| Renew Every | 3s | How often to renew lease |
| Poll Interval | 100ms | How often to check for new entries |
| Prune Margin | 1000 entries | How many entries to keep after pruning |
| Max Lag (Bounded) | 10000 entries | Maximum acceptable replica lag |

## Monitoring and Observability

Key metrics to track:
- **lag**: Current replication lag (primary_seq - replica_seq)
- **is_leader**: Boolean indicating if node is primary
- **entries_applied**: Total entries applied by replica
- **prune_count**: Number of times log was pruned
- **fencing_token**: Current fencing token (should be stable when single primary)

## Limitations and Future Work

### Current Limitations
- MemoryLeaseBackend not suitable for multi-node deployments
- No automatic replica discovery (must register manually)
- No automatic failover (requires manual promote or Consul/etcd)
- No state transfer on new replica join (replica must start empty)
- No compression of replication log

### Future Enhancements
- Automatic replica discovery via service mesh
- State transfer protocol for new replica bootstrap
- Compression of older log entries
- Snapshot-based replication for large states
- Adaptive consistency mode based on network latency
- Multi-level replication (cascade/hierarchical)
- Integrated monitoring dashboard
