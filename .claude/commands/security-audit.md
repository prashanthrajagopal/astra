# Security Audit

Perform a security review of Astra based on the platform security policy (S1-S6). Optionally specify a service or package to narrow scope.

## Checks

### S1 — Service-to-Service Authentication (mTLS)
Scan for inter-service communication without mTLS:
- gRPC clients without `credentials.NewTLS()` or `credentials.NewClientTLSFromCert()`
- HTTP calls between services without mutual TLS
- Files: `pkg/grpc/`, `cmd/*/main.go`, `internal/messaging/`

### S2 — External API Authentication (JWT)
Verify all external-facing endpoints require JWT:
- API gateway middleware validates JWT on every request
- Token expiry and signature validation
- Files: `cmd/api-gateway/`, `cmd/identity/`

### S3 — Authorization (RBAC / OPA)
Verify policy enforcement:
- Agent actions pass through `access-control` service
- Tool execution requests checked against OPA policies
- Files: `cmd/access-control/`, `internal/tools/`, `internal/agent/`

### S4 — Tool Sandbox Isolation
- WASM/Docker/Firecracker sandboxes with least privilege
- Network egress restricted, resource limits enforced
- Secrets via ephemeral volumes, not env vars
- Files: `internal/tools/`, `cmd/tool-runtime/`, `deployments/helm/`

### S5 — Secrets Management
- No API keys, passwords, or tokens in source code
- No secrets logged at any log level
- Vault integration for runtime secret injection
- Files: all `.go` files, `pkg/config/`, `deployments/`, `.gitignore`

### S6 — Approval Gates
- Dangerous tool executions require human approval
- Production deploys gated, data deletion gated
- Files: `internal/tools/`, `cmd/tool-runtime/`

### Dependency Vulnerabilities
Run: `govulncheck ./...`

## Output

| Rule | Status | Files | Details |
|------|--------|-------|---------|
| S1 (mTLS) | PASS/FAIL | {files} | {description} |
| S2 (JWT auth) | PASS/FAIL | {files} | {description} |
| S3 (RBAC/OPA) | PASS/FAIL | {files} | {description} |
| S4 (sandbox) | PASS/FAIL | {files} | {description} |
| S5 (secrets) | PASS/FAIL | {files} | {description} |
| S6 (approval gates) | PASS/FAIL | {files} | {description} |
| Dependencies | PASS/FAIL | — | {vulnerability count} |

**Any FAIL is blocking.**
