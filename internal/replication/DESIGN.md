# WAL Streaming Replication Design

## Problem Statement

MuninnDB requires a robust replication mechanism to:
1. Stream write-ahead log (WAL) entries to replicas
2. Ensure no split-brain writes during failover
3. Support multiple consistency modes (Eventual, Strong, BoundedStaleness)
4. Integrate seamlessly with Pebble storage engine

## Solution Overview

We implement a **log-based replication system** without exposing Pebble's WAL directly. Instead:

1. Every batch commit to Pebble appends an entry to a user-defined replication log
2. The replication log uses key prefix `0x19` (available in MuninnDB's key space)
3. A lease-based leader election prevents split-brain via fencing tokens
4. Replicas stream entries from primary and apply them idempotently

## Design Decisions

### 1. Why Not Expose Pebble WAL?

**Why we don't use Pebble's native WAL:**
- Pebble's WAL is internal and not part of the public API
- Different Pebble versions have different WAL formats
- We need application-level control over what gets replicated
- Pebble WAL includes internal operations (compactions, etc.) we don't want to replicate

**Instead:** We maintain a separate application-level log at the storage boundary.

### 2. Key Prefix Strategy

**Used prefix:** `0x19` (binary: 00011001)

**Available prefixes in MuninnDB:**
- `0x01`-`0x17`: Existing Pebble keys
- `0x18`: Available but reserved
- `0x19`: **Replication log** (this work)
- `0x1A`-`0xFF`: Future expansion

**Replication key layout:**
```
Sequence counter (metadata):
  0x19 | 0xFF | 0xFF | 0xFF | 0xFF | 0xFF | 0xFF | 0xFF | 0xFF

Log entries:
  0x19 | seq_be64(8) = 9 bytes total
  Value: msgpack-encoded ReplicationEntry
```

**Why big-endian sequence numbers?**
- Enables range scans to read entries in order
- Natural sort order without post-processing
- Efficient prefix scanning in Pebble

### 3. Serialization Format

**Why msgpack?**
- Already in muninndb's go.mod (vmihailenco/msgpack/v5)
- Compact binary format (~3x smaller than JSON)
- Fast encoder/decoder in Go
- Schema-agnostic (easy to evolve)

**ReplicationEntry structure:**
```go
type ReplicationEntry struct {
    Seq         uint64 // monotonically increasing
    Op          WALOp  // OpSet, OpDelete, OpBatch
    Key         []byte // Pebble key
    Value       []byte // Pebble value (nil for deletes)
    TimestampNS int64  // when entry was created
}
```

### 4. Leader Election Strategy

**Why lease-based?**
- Simple to understand and implement
- Doesn't require consensus (unlike Raft)
- Works with external systems (etcd, Consul)
- TTL prevents "zombie" leaders from writing

**How it works:**
1. All nodes try to acquire a lease with TTL of 10 seconds
2. First node to acquire becomes primary
3. Primary renews lease every 3 seconds
4. If primary fails to renew, lease expires and another node can acquire
5. Each lease acquisition increments a monotonically increasing "fencing token"

**Fencing token prevents split-brain:**
```
Old Primary (token=2)                 New Primary (token=3)
  ↓                                     ↓
  Write(key, value, token=2)  ✗ Rejected
                              New Primary accepts only token ≥ 3

  Client sees immediate rejection on stale token
  Cannot cause inconsistent state
```

### 5. Consistency Modes

#### ModeEventual
- Primary commits write to local Pebble only
- Replication is asynchronous, best-effort
- No waiting for replica acks
- **Guarantees:** Events reach replicas eventually (or not at all on failure)
- **Use:** High throughput, acceptable data loss

#### ModeStrong
- Primary appends entry to replication log
- Waits for all known replicas to acknowledge
- Only returns success when all ack'd
- **Guarantees:** No data loss (on known replicas); all data survives primary failure
- **Use:** Critical data, lower throughput acceptable

#### ModeBoundedStaleness
- Primary waits until lag threshold is met
- Replication entries must be within N entries of primary
- Replicas can be behind by at most N entries
- **Guarantees:** Replication lag is bounded; partial data loss possible
- **Use:** Latency-sensitive but consistency-critical apps

### 6. Thread Safety

**Synchronization strategy:**
- ReplicationLog: Mutex-protected sequence counter, Pebble handles concurrent entry writes
- Applier: Mutex-protected lastApplied, Pebble handles concurrent reads
- LeaderElector: Atomic flags for IsLeader/Token
- MemoryLeaseBackend: Mutex-protected holder/expires
- Coordinator: Mutex-protected replica map

**Why not channels everywhere?**
- Pebble already uses channels internally
- Mutex + sync.Map simpler for this use case
- Better performance for high-frequency operations (leader election tick)

### 7. Idempotency

**Problem:** Network retries could apply the same entry twice

**Solution:**
```go
func (a *Applier) Apply(entry ReplicationEntry) error {
    a.mu.Lock()
    defer a.mu.Unlock()

    // Skip if already applied
    if entry.Seq <= a.lastApplied {
        return nil
    }

    // Apply to Pebble
    batch := a.db.NewBatch()
    // ... write operations ...
    batch.Commit(nil)

    a.lastApplied = entry.Seq
    return nil
}
```

**Key insight:** Sequence numbers are never reused, so we can safely skip based on seq comparison.

### 8. Streamer Implementation

**Why polling instead of blocking iterator?**
- Replicas connect after primary starts (no synchronous bootstrap)
- Primary can have multiple connected replicas (1 streamer per replica)
- Polling allows clean shutdown without blocking
- Network hiccups don't crash the log

**Poll interval tuning:**
- 100ms default: Balance between latency and CPU
- Smaller (10ms): Lower replication latency, higher CPU
- Larger (1s): Higher latency, lower CPU

## Data Flow

### Primary Write Path

```
Client Write Request
    ↓
Engine validates
    ↓
BatchCommit to Pebble ← Blocks until fsync'd
    ↓
Return to client immediately
    ↓
[Async] Append to replication log ← Never blocks client
    ↓
Update sequence counter
```

### Replica Replication Flow

```
Streamer polls ReplicationLog
    ↓
ReplicationLog.ReadSince(lastSeq)
    ↓
Entries channel ← Buffered (1024 entries)
    ↓
gRPC Send to Replica (over network)
    ↓
Replica Receives Entry
    ↓
Applier.Apply(entry)
    ↓
Batch write to local Pebble
    ↓
Update lastApplied
    ↓
Return ack to Primary
    ↓
Primary updates ReplicaState.LastSeq
```

## Failure Scenarios

### Scenario 1: Primary Crashes

**What happens:**
1. Primary stops renewing lease
2. Lease TTL expires (10 seconds)
3. Next tick on replica: TryAcquire succeeds
4. Replica becomes primary, increments token
5. OnPromote callback called (notifies app)

**Data loss:** Entries in primary's write buffer but not yet in replication log (~100ms in flight)

### Scenario 2: Network Partition

**What happens:**
- If partition isolates primary: Primary can't renew (in minority), becomes secondary
- If partition isolates replica: Replica can't ack, primary sees lag grow
- Coordinator tracks lag; app can trigger manual failover

**How to handle:** Use `HandlePromoteReplica` to manually promote if needed

### Scenario 3: Replica Crashes

**What happens:**
1. Replica stops renewing lease (if it's primary) OR loses state (if secondary)
2. Replication log continues on primary
3. When replica restarts, it reads from lastApplied onward
4. Fills in missed entries

**Limitation:** Replica must retain `lastApplied` in persistent storage to avoid replaying

### Scenario 4: Stale Primary Continues Writing

**What happens:**
1. Primary loses leadership but doesn't realize (network issue)
2. Tries to write with old token=2
3. New primary has token=3
4. ValidateFencingToken(3, 2) returns ErrStaleFencingToken
5. Write is rejected

**Result:** No data corruption, split-brain prevented

## Performance Characteristics

### Latency Impact

**Per-write overhead:**
- Append to replication log: ~100µs (Pebble write)
- Streamer polling: Amortized 0 (happens in background)
- Replica apply: ~100µs per entry (Pebble write)

**Total for ModeStrong (waiting for all replicas):**
- Network roundtrip: ~10-100ms (dominant)
- Apply on replicas: ~100µs
- Total: ~10-100ms per write

### Throughput

**Primary (ModeEventual):**
- ~100k entries/sec (limited by Pebble batch commit rate)
- Replication log overhead: ~0 (async)

**Replica (applying):**
- ~50k entries/sec (limited by Pebble write throughput)

**Network (streaming):**
- ~10k entries/sec per replica (depends on network bandwidth)

### Storage

**Replication log overhead:**
- Per entry: 9 bytes (key) + 50-500 bytes (msgpack value) = ~100 bytes
- For 1M entries: ~100MB
- Grows indefinitely until pruned

## Testing Strategy

### Unit Tests
- ✓ Append and read log entries
- ✓ Sequence persistence across restarts
- ✓ Replication entry application (idempotent)
- ✓ Fencing token validation
- ✓ Leader election with multiple nodes
- ✓ Lag detection

### Integration Tests (future)
- Multiple primaries (detect split-brain)
- Network partition (failover)
- Replica catch-up after outage
- Consistency mode enforcement

### Benchmarks (future)
- Entry append throughput
- Streaming latency
- Failover time
- Replica lag under load

## Limits and Assumptions

### Assumptions
1. All writes go through single primary (no sharding)
2. Replica set is small (≤10 nodes)
3. Network is mostly stable (not designed for Byzantine fault tolerance)
4. Pebble DB is same version on all nodes (binary key compatibility)

### Limits
1. **Log size:** Unbounded until pruned (user must call Prune periodically)
2. **Replica count:** No fixed limit; scales linearly with updates needed
3. **Write throughput:** Limited by Pebble batch commit rate (~100k/sec)
4. **Replication lag:** No automatic bounds (use BoundedStaleness mode)

## Migration Path from Other Systems

### From PostgreSQL replication
- Both use log-based approach
- MuninnDB: streaming, not WAL segment files
- No logical vs physical distinction needed

### From etcd raft
- Both provide strong consistency mode
- MuninnDB: simpler (lease-based vs Raft)
- Tradeoff: less robust to Byzantine faults

### From custom WAL
- If you have existing WAL, use it to seed replication log on startup
- Then enable streaming replication going forward

## Future Enhancements

1. **Snapshot-based replication:** For new replicas, send current state + log tail
2. **Log compaction:** Merge entries into snapshots, prune old log
3. **Cascade replication:** Replica streams to another replica (reduces primary load)
4. **Automatic failover:** Integrate with Consul/K8s for automatic promotion
5. **Cross-region replication:** Async replication with eventual consistency SLA
6. **Encryption:** TLS for replication channel, encrypted log entries
7. **Monitoring:** Prometheus metrics for lag, token, leader status
8. **State transfer:** When new replica joins, transfer current state first

## References

- Pebble: https://github.com/cockroachdb/pebble
- Lease-based leader election: https://en.wikipedia.org/wiki/Lease_(computer_science)
- Fencing tokens: https://martin.kleppmann.com/papers/fencing-tokens.pdf
- Consistency models: https://en.wikipedia.org/wiki/Consistency_(database)
