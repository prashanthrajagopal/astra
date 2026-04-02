# Contributing to Astra

Thank you for your interest in contributing to Astra. This guide will help you get started.

## Code of Conduct

Be respectful, constructive, and professional. We are building production-grade infrastructure and hold ourselves to high standards in both code and conduct.

## Getting Started

### Prerequisites

- **Go 1.25+**
- **PostgreSQL 17** with pgvector extension
- **Redis 7+**
- **Memcached 1.6+**
- **Node.js 20+** (for E2E tests)
- **buf** (for protobuf generation)
- **Docker** (optional, for tool sandbox runtime)

### Local Setup

```bash
# Clone the repository
git clone https://github.com/prashanthrajagopal/astra.git
cd astra

# Start infrastructure (Postgres, Redis, Memcached)
# Uses native services when available, Docker fallback
./scripts/deploy.sh

# Run tests
go test ./... -count=1

# Run E2E tests (requires Astra running)
cd tests/e2e && npm install && npx playwright install
npm test
```

### Project Structure

```
cmd/                    # Service binaries (18 microservices)
internal/               # Private packages (kernel, actors, scheduler, etc.)
pkg/                    # Public packages (config, db, sdk, metrics, etc.)
proto/                  # Protobuf definitions
migrations/             # SQL migrations (idempotent)
tests/                  # Integration and E2E tests
docs/                   # PRD (single source of truth)
scripts/                # Deployment and utility scripts
deployments/            # Helm charts and Kubernetes manifests
```

## How to Contribute

### Reporting Issues

- Use GitHub Issues for bug reports and feature requests
- Include: steps to reproduce, expected vs actual behavior, Go/OS version
- For security vulnerabilities, email directly (do not open a public issue)

### Pull Requests

1. **Fork** the repository and create a feature branch from `main`
2. **Read the PRD** (`docs/PRD.md`) before implementing. The PRD is law.
3. **Follow existing patterns** — read the code around your change before writing
4. **Write tests** — table-driven tests for exported functions, integration tests for new features
5. **Run checks** before submitting:

```bash
go vet ./...
golangci-lint run ./...
go test ./... -count=1
```

6. **Keep PRs focused** — one feature or fix per PR
7. **Write clear commit messages** — imperative mood, explain why not what

### What We Look For

- **Clean architecture**: kernel -> internal -> cmd -> pkg
- **Production-ready code**: no TODOs, no prototypes
- **Error handling**: wrap errors with `fmt.Errorf("op: %w", err)`, never swallow
- **Structured logging**: `slog` only, never `fmt.Println`
- **Context propagation**: `context.Context` as first parameter on all I/O
- **Cache-first reads**: hot paths read from Redis/Memcached, never Postgres directly
- **Non-blocking actor sends**: `select` with `default`, return `ErrMailboxFull`

### Code Standards

#### Go

```go
// Context first
func DoSomething(ctx context.Context, id string) error {

// Wrap errors
if err != nil {
    return fmt.Errorf("doSomething: %w", err)
}

// Structured logging
slog.Info("operation completed", "id", id, "duration", elapsed)

// Table-driven tests
func TestFoo(t *testing.T) {
    tests := []struct {
        name string
        input string
        want  string
    }{...}
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {...})
    }
}
```

#### Protobuf

- Run `buf lint && buf generate` after any `.proto` changes
- Generated `.pb.go` files are committed

#### SQL Migrations

- Must be idempotent (`IF NOT EXISTS`, `ADD COLUMN IF NOT EXISTS`)
- Never drop columns without explicit approval
- Use `FOR UPDATE SKIP LOCKED` for task claiming
- Sequential numbering: `NNNN_description.sql`

### Performance Requirements

All contributions must respect these targets:

| Metric | Target |
|--------|--------|
| API read response (p99) | ≤ 10ms (from cache) |
| Task scheduling (median) | ≤ 50ms |
| Task scheduling (P95) | ≤ 500ms |
| Worker failure detection | ≤ 30s |

**Anti-patterns that will be rejected:**
- `db.Query` in read handlers (use cache)
- Synchronous LLM calls in hot paths
- Unbounded `KEYS`/`SCAN` on Redis
- Missing connection pools
- Blocking mailbox sends

### Security Policy

Violations are blocking — PRs will not be merged if they introduce:

- **S1**: Inter-service communication without TLS
- **S2**: External APIs without JWT auth
- **S3**: Operations bypassing OPA policy checks
- **S4**: Tool executions without sandbox (WASM/Docker/Firecracker)
- **S5**: Secrets in code, logs, or artifacts
- **S6**: Dangerous operations without approval gates

## Development Workflow

### Adding a New Service

1. Create `cmd/<service>/main.go` with health/ready endpoints
2. Add to `scripts/deploy.sh` build and start sections
3. Add to `deployments/helm/astra/templates/`
4. Update `docs/PRD.md` with service description
5. Add integration tests

### Adding a New Migration

1. Create `migrations/NNNN_description.sql`
2. Use idempotent DDL
3. Update `docs/PRD.md` schema section
4. Run `./scripts/deploy.sh` to verify migration applies cleanly

### Adding a New API Endpoint

1. Add handler in the appropriate service (`cmd/`)
2. Add proxy route in `cmd/api-gateway/main.go` if public
3. Update `cmd/api-gateway/dashboard/static/openapi.yaml`
4. Add E2E test in `tests/e2e/tests/api/`
5. Update `docs/PRD.md` API section

## Questions?

Open a GitHub Discussion or Issue. We're happy to help you get started.
