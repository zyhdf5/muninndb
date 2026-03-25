# REST API — Knowledge Base

**Subsystem:** HTTP/REST Transport  
**Complexity:** Medium (37 files)  
**Domain:** HTTP API, Middleware, Auth

## OVERVIEW

REST HTTP server implementing public API, admin endpoints, and cluster/replication routes. Uses middleware composition for auth, rate limiting, CORS, and request handling.

## STRUCTURE

```
internal/transport/rest/
├── server.go              # Main server & routing
├── admin_handlers.go     # Admin endpoints
├── replication_handlers.go # Replication routes
├── engine_adapter.go     # Engine API adapter
├── types.go              # Request/response types
├── errors.go             # Error handling
└── openapi.yaml          # OpenAPI spec
```

## WHERE TO LOOK

| Task | Location | Notes |
|------|----------|-------|
| Add endpoint | `server.go` | Route registration |
| Admin API | `admin_handlers.go` | Key/vault/plugin mgmt |
| Replication API | `replication_handlers.go` | Cluster routes |
| Error handling | `errors.go` | HTTP error mapping |
| API spec | `openapi.yaml` | OpenAPI contract |
| Middleware | `server.go` | Auth, rate limit, CORS |

## ROUTES

| Prefix | Purpose | Auth |
|--------|---------|------|
| /api/health | Health check | None |
| /api/hello | Handshake | None |
| /api/engrams | CRUD | Vault |
| /api/activate | Activation | Vault |
| /api/admin/* | Admin | Admin key |
| /v1/cluster/* | Cluster | Cluster token |

## BODY SIZE LIMITS

| Route Type | Limit |
|------------|-------|
| Public | 64KB |
| Authenticated | 4MB |
| Import/Export | 512MB |

## CONVENTIONS

### Middleware Chain
```
recovery → requestID → logging → bodySize → auth → rateLimit → handler
```

### Error Mapping
- 400: Invalid input
- 404: Not found
- 429: Rate limited
- 504: Activation timeout
- 500: Internal error (masked)

## ANTI-PATTERNS

- **NEVER** bypass auth middleware
- **NEVER** trust proxy headers for client IP
- **DO NOT** leak internal errors in responses
- **DO NOT** change middleware order without tests

## COMMANDS

```bash
# Run REST tests
go test ./internal/transport/rest/... -v

# Check OpenAPI spec
make api-spec-validation
```

## NOTES

- **Rate Limiting**: Token bucket per IP
- **CORS**: Allowlist-based
- **Request ID**: Tracing support
- **Recovery**: Panic recovery with stack trace
