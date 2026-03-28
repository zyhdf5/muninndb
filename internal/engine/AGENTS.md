# Engine — Knowledge Base

**Subsystem:** Cognitive Engine  
**Complexity:** High (65 files)  
**Domain:** Activation, Scoring, Cognitive Workers

## OVERVIEW

Core cognitive engine implementing temporal learning, Hebbian association, confidence scoring, and activation propagation. Orchestrates storage, indexing, and background cognitive workers.

## STRUCTURE

```
internal/engine/
├── engine.go              # Main engine orchestration
├── activation/            # Activation pipeline
│   └── engine.go         # Scoring & propagation
├── trigger/              # Trigger system
├── autoassoc/            # Auto-association
├── tree.go               # Tree/ordinal operations
├── engine_replay.go      # Background replay
├── engine_reembed.go     # Re-embedding orchestration
└── config.go             # Engine configuration
```

## WHERE TO LOOK

| Task | Location | Notes |
|------|----------|-------|
| Add activation feature | `activation/engine.go` | Scoring pipeline |
| Add trigger | `trigger/registry.go` | Trigger dispatch |
| Tree operations | `tree.go` | RememberTree, AddChild |
| Background workers | `engine.go` | Worker lifecycle |
| Re-embedding | `engine_reembed.go` | Retroactive enrichment |
| Engine config | `config.go` | Limits & tuning |

## COGNITIVE WORKERS

| Worker | Purpose | Location |
|--------|---------|----------|
| HebbianWorker | Co-activation learning | `../cognitive/hebbian.go` |
| TransitionWorker | Sequential patterns | `../cognitive/transition.go` |
| ContradictionWorker | Conflict detection | `../cognitive/contradict.go` |
| ConfidenceWorker | Confidence scoring | `../cognitive/confidence.go` |
| ActivityWorker | Activity tracking | `../cognitive/activity.go` |

## WRITE FLOW

1. **Engine.Write** builds Engram
2. **store.WriteEngram** — atomic Pebble batch
3. **Post-commit** (async):
   - WAL/MOL append (if configured)
   - FTS indexing
   - HNSW insert
   - Provenance append
   - Hebbian/Transition/Contradiction events
   - Novelty detection
   - Auto-association
   - Activity update

## ACTIVATION FLOW

1. Query relevance buckets (0x10)
2. FTS search
3. HNSW vector search
4. Association traversal (0x03/0x04)
5. Hebbian boosts
6. Transition predictions
7. Temporal scoring (Ebbinghaus)
8. Return activations

## CONVENTIONS

### Worker Lifecycle
- Workers are hot-swappable for cluster roles (Cortex vs Lobe)
- Use `cogMu` mutex for safe worker pointer updates
- Context cancellation for graceful shutdown

### Event Processing
- Co-activation events → HebbianWorker
- Transition events → TransitionWorker
- Contradiction events → ConfidenceWorker

### Working Memory
- Decay: `CurrentAttention = Attention0 × 2^(-elapsed/halfLife)`
- Promotion threshold: attention > 0.6
- Promotion delta: `min(attention × 0.1, 0.15)`

## ANTI-PATTERNS

- **NEVER** block in cognitive workers — use channels
- **NEVER** modify worker pointers without `cogMu`
- **DO NOT** skip event batching in workers
- **DO NOT** call storage directly — use EngineStore interface

## COMMANDS

```bash
# Run engine tests
go test ./internal/engine/... -v

# Run activation tests
go test ./internal/engine/activation/... -v

# Benchmark activation
make bench
```

## NOTES

- **Ebbinghaus**: Forgetting curve implemented in decay
- **Hebbian**: Multiplicative weight updates in log-space
- **PAS**: Predictive activation via transition counts
- **Cluster Roles**: Lobe (storage) vs Cortex (compute)
- **Hot-swap**: Workers can be swapped without restart
