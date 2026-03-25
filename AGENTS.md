# MuninnDB — Project Knowledge Base

**Generated:** 2025-03-24  
**Language:** Go 1.25  
**Type:** Cognitive Database with Multi-Protocol Support

## OVERVIEW

MuninnDB is a cognitive memory database implementing temporal learning, Hebbian association, and multi-modal retrieval (vector + full-text + graph). Supports REST, gRPC, MCP (Model Context Protocol), and custom MBP binary protocol.

## STRUCTURE

```
.
├── cmd/                    # CLI entry points
│   ├── muninn/            # Main server binary
│   ├── diag/              # Diagnostic tools
│   └── bench/             # Benchmark suite
├── internal/              # Core implementation
│   ├── storage/           # Persistence layer (Pebble, ERF, WAL)
│   ├── engine/            # Cognitive engine (activation, scoring)
│   ├── replication/       # Clustering & replication
│   ├── transport/         # Protocol adapters
│   │   ├── rest/         # HTTP/REST API
│   │   ├── grpc/         # gRPC service
│   │   └── mbp/          # Binary protocol
│   ├── mcp/              # Model Context Protocol
│   ├── plugin/           # Plugin system (embed/enrich)
│   ├── auth/             # Authentication & vaults
│   ├── cognitive/        # Cognitive algorithms
│   ├── index/            # Indexes (HNSW, FTS, adjacency)
│   └── wal/              # Write-ahead log
├── sdk/                   # Client SDKs
│   ├── go/               # Go SDK
│   ├── python/           # Python SDK
│   ├── node/             # Node.js/TypeScript SDK
│   ├── kotlin/           # Kotlin SDK
│   ├── php/              # PHP SDK
│   └── swift/            # Swift SDK
├── proto/                 # Protobuf definitions
├── web/                   # Web UI assets
└── docs/                  # Documentation
```

## WHERE TO LOOK

| Task | Location | Notes |
|------|----------|-------|
| Add CLI command | `cmd/muninn/main.go` | Subcommand dispatch |
| Add REST endpoint | `internal/transport/rest/server.go` | Route registration |
| Add gRPC method | `proto/muninn/v1/service.proto` | Define + regenerate |
| Add MCP tool | `internal/mcp/handlers.go` | Tool dispatch map |
| Change storage format | `internal/storage/erf/` | Custom ERF format |
| Add embedding provider | `internal/plugin/embed/` | Implement EmbedPlugin |
| Add LLM enricher | `internal/plugin/enrich/` | Implement EnrichPlugin |
| Change auth behavior | `internal/auth/middleware.go` | Middleware & guards |
| Cluster operations | `internal/replication/coordinator.go` | Leader election |
| Cognitive algorithms | `internal/cognitive/` | Hebbian, decay, confidence |

## CONVENTIONS

### Build Requirements
- **MUST** use `-tags localassets` when building muninn binary (enables embedded ONNX embedder)
- Run `make fetch-assets` before first build (downloads model + ORT libs)
- Web assets: `make web` or `make css` before build

### Code Organization
- Package per subsystem under `internal/`
- Transport adapters are thin: validate → map → call EngineAPI
- Engine exposes `EngineAPI` interface consumed by all transports
- Storage uses façade pattern: `store.go` abstracts Pebble/ERF/WAL

### Error Handling
- Use wrapped errors with context: `fmt.Errorf("...: %w", err)`
- Transport layers map internal errors to appropriate HTTP/gRPC status codes
- Storage errors often wrapped with operation context

### Testing
- Test files: `*_test.go` alongside source
- Integration tests: `*_integration_test.go` with `//go:build integration`
- Benchmarks: `*_benchmark_test.go` or `cmd/bench/`

### Configuration
- Server config: flags + env vars (see `cmd/muninn/main.go`)
- Plugin config: `plugin_config.json` in data directory
- Cluster config: managed by coordinator, stored in Pebble

## ANTI-PATTERNS

- **NEVER** build without `-tags localassets` — breaks local embedder
- **NEVER** modify ERF format without migration plan in `internal/storage/migrate/`
- **NEVER** bypass auth middleware in transport handlers
- **NEVER** call storage directly from transports — go through EngineAPI
- **NEVER** ignore WAL errors — data loss risk

## COMMANDS

```bash
# Setup (one-time)
make fetch-assets          # Download model + ORT libraries

# Build
make build                 # Build server binary (with web assets)
go build -tags localassets -o muninndb-server ./cmd/muninn/...

# Test
make test                  # Unit tests
make test-integration      # Integration tests (needs clean environment)
make bench                 # Benchmarks

# Docker
docker compose up --build -d
docker build -t muninndb:latest .

# Development
make web                   # Build web UI assets
make css                   # Build CSS only (faster)
make clean-assets          # Remove downloaded assets
```

## PORTS

| Service | Port | Protocol |
|---------|------|----------|
| MBP | 8474 | Binary |
| REST | 8475 | HTTP |
| Web UI | 8476 | HTTP |
| gRPC | 8477 | HTTP/2 |
| MCP | 8750 | HTTP/SSE |

## NOTES

- **Cognitive Engine**: Implements Ebbinghaus forgetting curve, Hebbian learning, and confidence scoring. Not a simple CRUD database.
- **ERF Format**: Custom append-only record format for certain workloads. See `internal/storage/erf/`
- **Single Binary**: Embeds platform-specific ONNX Runtime libraries. Build tags control inclusion.
- **MCP Protocol**: AI-agent integration via JSON-RPC 2.0 + SSE. Tools defined in `internal/mcp/handlers.go`
- **Replication**: WAL-based streaming replication. Leader election via coordinator.

## SUBSYSTEM DOCUMENTATION

| Subsystem | AGENTS.md |
|-----------|-----------|
| Storage | `internal/storage/AGENTS.md` |
| Engine | `internal/engine/AGENTS.md` |
| Replication | `internal/replication/AGENTS.md` |
| Plugin | `internal/plugin/AGENTS.md` |
| MCP | `internal/mcp/AGENTS.md` |
| REST API | `internal/transport/rest/AGENTS.md` |
| Auth | `internal/auth/AGENTS.md` |
| Cognitive | `internal/cognitive/AGENTS.md` |
