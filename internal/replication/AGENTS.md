# Replication — Knowledge Base

**Subsystem:** Clustering & Replication  
**Complexity:** High (47 files)  
**Domain:** Distributed Systems, Consensus, Streaming

## OVERVIEW

WAL-based streaming replication with leader election, cluster coordination, and multi-node consistency. Implements a coordinator-based cluster model with Lobe (storage) and Cortex (compute) roles.

## STRUCTURE

```
internal/replication/
├── coordinator.go         # Leader election & cluster mgmt
├── streamer.go           # WAL streaming
├── applier.go            # Apply replicated ops
├── leader.go             # Leader-specific logic
├── join.go               # Node join/leave
├── conn_manager.go       # Connection management
├── DESIGN.md             # Architecture design
└── INTEGRATION.md        # Integration guide
```

## WHERE TO LOOK

| Task | Location | Notes |
|------|----------|-------|
| Leader election | `coordinator.go` | Cluster coordination |
| Streaming | `streamer.go` | WAL streaming to followers |
| Apply changes | `applier.go` | Follower-side apply |
| Join cluster | `join.go` | Node discovery |
| Connections | `conn_manager.go` | Connection pooling |
| Cluster config | `DESIGN.md` | Architecture decisions |

## CONVENTIONS

### Cluster Roles
- **Cortex**: Compute nodes, run cognitive workers
- **Lobe**: Storage nodes, persist data
- Nodes can be both (hybrid mode)

### Replication Flow
1. Leader writes to local WAL
2. Streamer reads WAL, sends to followers
3. Followers apply via Applier
4. Fencing/epoch for safety

## ANTI-PATTERNS

- **NEVER** bypass coordinator for cluster changes
- **NEVER** ignore epoch/fencing checks
- **DO NOT** mix sync/async replication without config

## NOTES

- **WAL Streaming**: Based on MOL segments
- **Fencing**: Epoch-based safety
- **Lease**: Leader lease for availability
- **Snapshot**: Initial sync via snapshots
