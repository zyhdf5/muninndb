# Plugin System — Knowledge Base

**Subsystem:** Plugin Architecture  
**Complexity:** High (35+ files)  
**Domain:** Embeddings, Enrichment, LLM Integration

## OVERVIEW

Pluggable architecture for embedding providers (vectorization) and enrichment providers (LLM). Supports local ONNX, Ollama, OpenAI, and other providers via unified interfaces.

## STRUCTURE

```
internal/plugin/
├── plugin.go              # Plugin interfaces
├── registry.go           # Plugin registry
├── provider.go           # Provider plumbing
├── embed/                # Embedding providers
│   ├── local.go         # Local ONNX embedder
│   ├── ollama.go        # Ollama adapter
│   └── adapters.go      # Provider adapters
├── enrich/               # Enrichment providers
│   ├── enrich.go        # Enrichment pipeline
│   └── pipeline.go      # Async workers
└── llmstats/            # LLM metrics
```

## WHERE TO LOOK

| Task | Location | Notes |
|------|----------|-------|
| Add embed provider | `embed/adapters.go` | Implement EmbedPlugin |
| Add enrich provider | `enrich/enrich.go` | Implement EnrichPlugin |
| Plugin lifecycle | `registry.go` | Registration & init |
| Local embedder | `embed/local.go` | ONNX Runtime |
| Enrichment pipeline | `enrich/pipeline.go` | Async workers |
| LLM metrics | `llmstats/llmstats.go` | Usage tracking |

## PROVIDER PRIORITY

1. Environment variables
2. Saved plugin_config.json
3. Bundled local ONNX
4. Noop fallback

## CONVENTIONS

### Plugin Interface
```go
type Plugin interface {
    Init(ctx context.Context, cfg PluginConfig) error
    Close() error
}
```

### EmbedPlugin
```go
type EmbedPlugin interface {
    Plugin
    Embed(ctx context.Context, text string) ([]float32, error)
    Dimension() int
}
```

### EnrichPlugin
```go
type EnrichPlugin interface {
    Plugin
    Enrich(ctx context.Context, engram *Engram) error
}
```

## ANTI-PATTERNS

- **NEVER** build without `-tags localassets` — breaks local embedder
- **NEVER** ignore provider rate limits
- **DO NOT** block in plugin Init
- **DO NOT** skip credential validation

## COMMANDS

```bash
# Fetch assets (required)
make fetch-assets

# Build with local embedder
go build -tags localassets ./cmd/muninn/...

# Run plugin tests
go test ./internal/plugin/... -v
```

## NOTES

- **Local Embedder**: ONNX Runtime with embedded models
- **Assets**: Downloaded to `internal/plugin/embed/assets/`
- **Retroactive**: Background enrichment of existing engrams
- **Circuit Breaker**: Enrichment has circuit breaker pattern
