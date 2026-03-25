# MCP — Knowledge Base

**Subsystem:** Model Context Protocol  
**Complexity:** Medium (32 files)  
**Domain:** AI Agent Integration, JSON-RPC, SSE

## OVERVIEW

MCP (Model Context Protocol) server implementing JSON-RPC 2.0 over HTTP/SSE for AI agent integration. Exposes MuninnDB as a set of tools for LLM agents.

## STRUCTURE

```
internal/mcp/
├── server.go              # MCP HTTP server
├── handlers.go           # Tool handlers
├── engine_adapter.go     # Engine API adapter
├── session.go            # Session management
├── eventbus.go           # Event bus
├── types.go              # MCP types
└── guide.go              # Usage guide
```

## WHERE TO LOOK

| Task | Location | Notes |
|------|----------|-------|
| Add MCP tool | `handlers.go` | Tool dispatch map |
| Session mgmt | `session.go` | SSE sessions |
| Engine adapter | `engine_adapter.go` | Request mapping |
| Event handling | `eventbus.go` | Async events |
| Server config | `server.go` | Routes & middleware |

## ENDPOINTS

| Endpoint | Method | Purpose |
|----------|--------|---------|
| /mcp | POST | JSON-RPC |
| /mcp | GET | SSE transport |
| /mcp/message | POST | SSE message POST |
| /mcp/tools | GET | List tools |
| /mcp/health | GET | Health check |

## TOOLS

| Tool | Handler | Purpose |
|------|---------|---------|
| muninn_remember | handleRemember | Store engram |
| muninn_recall | handleRecall | Retrieve engrams |
| muninn_guide | handleGuide | Usage guide |
| muninn_activate | handleActivate | Activation |
| muninn_batch | handleBatch | Batch operations |

## CONVENTIONS

### Tool Handler Pattern
```go
func handleRemember(ctx context.Context, args map[string]interface{}) (interface{}, error)
```

### Session Lifecycle
1. Client opens SSE stream
2. Server creates session
3. Client POSTs messages to session endpoint
4. Server dispatches to handlers
5. Session timeout/cleanup

## ANTI-PATTERNS

- **NEVER** bypass auth in MCP handlers
- **NEVER** leak session tokens in logs
- **DO NOT** block SSE streams
- **DO NOT** ignore idempotency keys

## COMMANDS

```bash
# Run MCP tests
go test ./internal/mcp/... -v

# Test MCP stdio
./muninndb-server mcp
```

## NOTES

- **Transport**: HTTP POST or SSE
- **Auth**: Static bearer (mdb_) or API key (mk_)
- **Idempotency**: Supported via idempotency keys
- **Session Pinning**: Vault pinned to API key sessions
