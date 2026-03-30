# Auto-Associations

## What it does

When you write an engram with tags, MuninnDB automatically finds existing engrams that share one or more of those tags and creates `RELATES_TO` associations between them — without any additional API calls from the client.

## Why it matters

Traditional databases require you to explicitly model every relationship. MuninnDB infers relationships from shared concepts at write time, so the association graph grows organically as you store memories. An engram about "neural plasticity" tagged with `["neuroscience", "learning"]` will automatically link to every other engram in the vault that touches either topic.

## How it works

1. After writing the new engram to storage, MuninnDB enqueues an auto-association job.
2. A bounded worker pool (4 goroutines, 1024-job buffer) picks up the job asynchronously — it **never blocks the write response**.
3. For up to 5 of the engram's tags (chosen by position, most specific first), the FTS index is queried to find matching engrams.
4. Results are deduplicated and capped at 10 candidates.
5. A `RELATES_TO` link (weight=0.3) is created from the new engram to each candidate.
6. If the queue is full under sustained load, jobs are dropped (with metrics) rather than blocking writes.

## What is new / never done before (to our knowledge)

Most databases require explicit join tables or graph edges declared by the client. MuninnDB creates those edges at write time using the FTS index, making the memory graph self-organizing. The bounded worker pool + backpressure design ensures this never degrades write latency even at high throughput.

## Configuration

No configuration required. Auto-association is always enabled. Tuning constants in `internal/engine/autoassoc/autoassoc.go`:

| Constant | Default | Description |
|----------|---------|-------------|
| `MaxTagQueries` | 5 | Max tag searches per write |
| `MaxAssociations` | 10 | Max links created per write |
| `AssocWeight` | 0.3 | Link weight (lower = weaker than manual links) |
| `JobBufSize` | 1024 | Worker queue capacity |
| `NumWorkers` | 4 | Worker goroutines |

## Observability

```go
enqueued, completed, dropped, errors := engine.autoAssoc.GetMetrics()
```

- **dropped > 0** means write throughput exceeded worker capacity. Increase `NumWorkers` or `JobBufSize`.
- **errors > 0** means FTS or storage returned errors; check application logs.
