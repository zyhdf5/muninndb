# Storage — Knowledge Base

**Subsystem:** Persistence Layer  
**Complexity:** High (78 files)  
**Domain:** Durability, Key-Value Storage, WAL, Migrations

## OVERVIEW

Storage layer implements a hybrid persistence system using Pebble (embedded LSM), custom ERF (append-only record format), and WAL (write-ahead log). Supports atomic batches, snapshots, migrations, and replication-ready WAL streaming.

## STRUCTURE

```
internal/storage/
├── store.go              # EngineStore interface
├── impl.go               # PebbleStore implementation
├── pebble.go             # Pebble helpers
├── types.go              # Core data types (Engram, ULID)
├── engram.go             # Engram CRUD operations
├── entity.go             # Entity graph keyspace
├── transition.go         # Transition store
├── erf/                  # Custom ERF format
├── migrate/              # Migration pipeline
└── wal_syncer.go         # Group-commit durability
```

## WHERE TO LOOK

| Task | Location | Notes |
|------|----------|-------|
| Add storage backend | `store.go` | Implement EngineStore |
| Modify ERF format | `erf/encode.go` | Custom binary format |
| Add migration | `migrate/migrate.go` | On-disk format upgrades |
| Change durability | `impl.go`, `wal_syncer.go` | Sync vs NoSync |
| Entity operations | `entity.go` | Graph keyspace (0x20/0x21/0x23/0x24) |
| Engram lifecycle | `engram.go` | 0x01 (full), 0x02 (meta) |
| Atomic batches | `impl.go:NewBatch()` | StoreBatch interface |

## KEYSPACE MAP

| Prefix | Purpose | Key Pattern |
|--------|---------|-------------|
| 0x01 | Full engram | `{ws}01{id}` |
| 0x02 | Metadata | `{ws}02{id}` |
| 0x03 | Forward assoc | `{ws}03{from}{to}` |
| 0x04 | Reverse assoc | `{ws}04{to}{from}` |
| 0x05/0x06 | FTS postings | `{ws}05{term}` |
| 0x10 | Relevance buckets | `{ws}10{relevance}{id}` |
| 0x14 | Assoc weights | `{ws}14{from}{to}` |
| 0x18 | Embeddings | `{ws}18{id}` |
| 0x1E | Ordinals | `{ws}1E{parent}{ordinal}` |
| 0x1F | Entity records | `{ws}1F{entity}` |
| 0x20 | Entity→engram links | `{ws}20{entity}{id}` |
| 0x21 | Relationships | `{ws}21{rel}` |
| 0x22 | Last access | `{ws}22{id}` |
| 0x23 | Reverse entity links | `{ws}23{id}{entity}` |
| 0x24 | Co-occurrence | `{ws}24{e1}{e2}` |

## CONVENTIONS

### Durability Contract
- **Default**: `pebble.Sync` — immediate fsync for critical writes
- **NoSync mode**: `pebble.NoSync` + `walSyncer` — group fsync every 10ms
- **WAL**: MOL append-only log with `GroupCommitter` for batch fsync

### Atomic Operations
- `StoreBatch` for multi-engram atomic writes
- `RelinkEntityEngramLink` for safe entity relinking
- `RememberTree` / `AddChild` for atomic tree operations

### Caching Strategy
- **Intentionally NOT caching on write** to avoid flooding L1 cache
- L1 cache for reads only; metaCache for metadata
- assocCache for association lookups

## ANTI-PATTERNS

- **NEVER** modify ERF format without migration plan
- **NEVER** call storage directly from transports — use EngineAPI
- **NEVER** ignore WAL errors — data loss risk
- **DO NOT** trust proxy headers for rate limiting
- **DO NOT** cache writes in hot loops

## COMMANDS

```bash
# Run storage tests
go test ./internal/storage/... -v

# Run with race detector
go test ./internal/storage/... -race

# Integration tests (needs clean env)
go test -tags integration ./internal/storage/...
```

## NOTES

- **ERF Format**: Custom append-only record format. See `erf/encode.go`
- **MOL WAL**: Separate from Pebble WAL. Used for replication streaming
- **GroupCommitter**: Batches MOL appends, single fsync per group
- **Migrations**: Versioned on-disk format upgrades in `migrate/`
- **Snapshots**: Point-in-time exports via `snapshot.go`
