# Security Audit

Perform a security review of Astra based on the platform security policy (S1-S6). The user may optionally specify a service or package to narrow scope.

## Pre-requisites — Read Before Acting

1. Read `.cursor/rules/SECURITY-RULE.mdc` for the full security policy (S1-S6)

## Delegation

Delegate to **Architect** for the audit. The Architect reports findings and delegates fixes through **Tech Lead**.

## Checks

### S1 — Service-to-Service Authentication (mTLS)

Scan for inter-service communication without mTLS:
- gRPC clients without `credentials.NewTLS()` or `credentials.NewClientTLSFromCert()`
- HTTP calls between services without mutual TLS
- Hardcoded service addresses without certificate validation

Files to scan:
- `pkg/grpc/` — gRPC helpers and interceptors
- `cmd/*/main.go` — service entrypoints (client connections)
- `internal/messaging/` — Redis client TLS configuration

### S2 — External API Authentication (JWT)

Verify all external-facing endpoints require JWT:
- API gateway middleware validates JWT on every request
- Token expiry and signature validation
- Service-issued tokens for inter-service calls

Files to scan:
- `cmd/api-gateway/` — auth middleware
- `cmd/identity/` — token issuance and validation

### S3 — Authorization (RBAC / OPA)

Verify policy enforcement:
- Agent actions pass through `access-control` service
- Tool execution requests checked against OPA policies
- Per-agent permission scopes enforced

Files to scan:
- `cmd/access-control/` — OPA integration
- `internal/tools/` — tool permission checks
- `internal/agent/` — agent scope enforcement

### S4 — Tool Sandbox Isolation

Verify sandbox security:
- WASM/Docker/Firecracker sandboxes configured with least privilege
- Network egress restricted (no unrestricted outbound)
- Resource limits enforced (CPU, memory, disk, time)
- Secrets delivered via ephemeral volumes, not env vars

Files to scan:
- `internal/tools/` — sandbox lifecycle
- `cmd/tool-runtime/` — tool runtime service
- `deployments/helm/` — network policies

### S5 — Secrets Management

Scan for secret exposure:
- No API keys, passwords, or tokens in source code
- No secrets logged at any log level (check `slog` calls)
- No secrets in Postgres `events` table or artifact `metadata`
- Vault integration for runtime secret injection

Files to scan:
- All `.go` files — hardcoded strings matching key/token/password patterns
- `pkg/config/` — config loading (should use Vault, not plaintext)
- `deployments/` — Helm values (no secrets in values.yaml)
- `.gitignore` — must include `.env`, `*.pem`, `*.key`

### S6 — Approval Gates

Verify dangerous operations are gated:
- Tool executions flagged as dangerous require human approval
- Production deploys gated
- Data deletion operations gated

Files to scan:
- `internal/tools/` — policy engine intercept
- `cmd/tool-runtime/` — approval flow integration

### Dependency Vulnerabilities

Run:
```bash
go list -m all | nancy sleuth
# or
govulncheck ./...
```

## Output

| Rule | Status | Files | Details |
|------|--------|-------|---------|
| S1 (mTLS) | PASS/FAIL | {files} | {description} |
| S2 (JWT auth) | PASS/FAIL | {files} | {description} |
| S3 (RBAC/OPA) | PASS/FAIL | {files} | {description} |
| S4 (sandbox isolation) | PASS/FAIL | {files} | {description} |
| S5 (secrets mgmt) | PASS/FAIL | {files} | {description} |
| S6 (approval gates) | PASS/FAIL | {files} | {description} |
| Dependencies | PASS/FAIL | — | {vulnerability count by severity} |

**Any FAIL is blocking.** Route fixes through Tech Lead → Go Engineer.
