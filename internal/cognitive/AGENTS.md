# Cognitive — Knowledge Base

**Subsystem:** Cognitive Algorithms  
**Complexity:** Medium (17 files)  
**Domain:** Learning, Decay, Confidence

## OVERVIEW

Cognitive algorithms implementing Hebbian learning, Ebbinghaus forgetting curve, confidence scoring, and activity tracking. Background workers process events asynchronously.

## STRUCTURE

```
internal/cognitive/
├── worker.go              # Worker orchestration
├── hebbian.go            # Hebbian learning
├── decay.go              # Ebbinghaus decay
├── confidence.go         # Confidence scoring
├── contradict.go         # Contradiction detection
├── activity.go           # Activity tracking
├── transition.go         # Transition learning
├── coactivity.go         # Co-activity tracking
└── store_adapters.go     # Storage adapters
```

## WHERE TO LOOK

| Task | Location | Notes |
|------|----------|-------|
| Hebbian learning | `hebbian.go` | Weight updates |
| Decay | `decay.go` | Forgetting curve |
| Confidence | `confidence.go` | Confidence scoring |
| Contradiction | `contradict.go` | Conflict detection |
| Activity | `activity.go` | Activity tracking |
| Transitions | `transition.go` | Sequential patterns |
| Worker lifecycle | `worker.go` | Background workers |

## ALGORITHMS

### Hebbian Learning
- Co-activation events trigger weight updates
- Multiplicative updates in log-space
- Batch processing for efficiency

### Ebbinghaus Decay
```
Relevance(t) = Relevance0 × 2^(-elapsed / stability)
```

### Confidence Scoring
- Based on contradiction detection
- Propagates through associations
- Affects activation ranking

## CONVENTIONS

### Worker Pattern
```go
type Worker struct {
    events chan Event
    store  storage.EngineStore
}

func (w *Worker) Run(ctx context.Context)
func (w *Worker) ProcessBatch(batch []Event)
```

### Event Batching
- Collect events in channel
- Process in batches
- Update storage atomically

## ANTI-PATTERNS

- **NEVER** block event channels
- **NEVER** skip batch processing
- **DO NOT** modify weights without locking
- **DO NOT** ignore context cancellation

## COMMANDS

```bash
# Run cognitive tests
go test ./internal/cognitive/... -v
```

## NOTES

- **Hebbian**: "Neurons that fire together, wire together"
- **Decay**: Ebbinghaus forgetting curve
- **PAS**: Predictive activation system
- **Workers**: Run as background goroutines
- **Hot-swap**: Workers can be replaced
