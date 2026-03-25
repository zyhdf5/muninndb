# Auth — Knowledge Base

**Subsystem:** Authentication & Authorization  
**Complexity:** Medium (18 files)  
**Domain:** Security, Vaults, API Keys

## OVERVIEW

Authentication and authorization system supporting API keys, vault-scoped access, and cluster tokens. Implements middleware for REST and interceptors for gRPC.

## STRUCTURE

```
internal/auth/
├── middleware.go          # HTTP middleware
├── keys.go               # Key handling
├── keys_store.go         # Key storage
├── bootstrap.go          # Initial admin setup
├── vault_config.go       # Vault configuration
└── types.go              # Auth types
```

## WHERE TO LOOK

| Task | Location | Notes |
|------|----------|-------|
| Add auth method | `middleware.go` | Middleware chain |
| Key management | `keys_store.go` | CRUD operations |
| Bootstrap | `bootstrap.go` | Initial setup |
| Vault config | `vault_config.go` | Vault settings |
| Key crypto | `keys.go` | Signing/verification |

## AUTH MODEL

| Token Type | Prefix | Purpose |
|------------|--------|---------|
| API Key | mk_ | User access |
| Admin Key | ma_ | Admin operations |
| Cluster Token | mc_ | Cluster comms |
| MCP Static | mdb_ | MCP auth |

## VAULT MODEL

- Each vault has isolated namespace
- API keys scoped to vaults
- Default vault for unauthenticated
- Vault-level quotas and limits

## CONVENTIONS

### Middleware Order
```
recovery → requestID → logging → auth → rateLimit → handler
```

### Key Validation
- Constant-time comparison
- Prefix validation
- Expiration checks
- Vault scoping

## ANTI-PATTERNS

- **NEVER** log full API keys
- **NEVER** use timing-vulnerable comparisons
- **DO NOT** bypass vault scoping
- **DO NOT** hardcode keys in code

## COMMANDS

```bash
# Run auth tests
go test ./internal/auth/... -v
```

## NOTES

- **Bootstrap**: First admin key created on init
- **Rotation**: Keys can be revoked/rotated
- **Storage**: Keys stored in Pebble
- **Cluster**: Separate cluster token for replication
