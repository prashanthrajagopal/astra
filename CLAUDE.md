# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Astra is a production-grade, microkernel-style autonomous agent operating system written in Go. It's a distributed system with 18 services, an actor-based microkernel, and planet-scale targets (millions of agents, 100M+ tasks/day). The PRD is the source of truth: `docs/PRD.md`.

## Build & Development Commands

```bash
# Build a single service
go build -o bin/SERVICE ./cmd/SERVICE

# Build all services (what deploy.sh does)
for svc in cmd/*/; do go build -o bin/$(basename $svc) ./$svc; done

# Run tests
go test ./... -count=1          # All tests
go test -short ./...            # Unit only (skip integration)
go test ./internal/tasks -v     # Single package
go test ./internal/tasks -run TestName -v  # Single test

# Lint (required after every change)
go vet ./...
golangci-lint run ./internal/tasks/... ./cmd/task-service/...  # Changed packages only

# Proto generation (after .proto changes)
buf lint && buf generate

# Local deployment (starts Postgres/Redis/Memcached via Docker if missing)
./scripts/deploy.sh

# Stop local services
for f in logs/*.pid; do kill $(cat $f) 2>/dev/null; done

# GCP deployment
./scripts/gcp-deploy.sh --setup --dev   # First time
./scripts/gcp-deploy.sh --dev           # Subsequent deploys

# Seed default agents
./scripts/seed-agents.sh
```

## Architecture

### Microkernel (`internal/kernel/`)

The kernel is a simple actor registry. `Spawn(actor)` registers, `Send(ctx, target, msg)` routes messages, `Stop(id)` tears down. All inter-service coordination goes through the kernel.

### Actor Framework (`internal/actors/`)

Every service component is an Actor with a buffered mailbox (channel of 1024). Key rules:
- Sends are **non-blocking** (`select` with `default`), returning `ErrMailboxFull` if the channel is full
- Actors start a goroutine loop via `Start(handler)`, shut down via stop channel + WaitGroup
- `Message` carries `Type`, `Source`, `Target`, and `json.RawMessage` payload

### Messaging Bus (`internal/messaging/`)

Redis Streams (not Kafka) with consumer groups. Features auto-retry with reclaim (3 max retries, 30s min idle) and a dead-letter stream (`astra:dead_letter`). API: `Publish`, `Consume`, `ConsumeWithOptions`.

### Task Scheduling (`internal/scheduler/`, `internal/tasks/`)

Tasks shard by `hash(agent_id) % TASK_SHARD_COUNT` to Redis streams (`astra:tasks:shard:{i}`). Ready-task detection uses `NOT EXISTS` subqueries against dependency tables. Task claiming uses `FOR UPDATE SKIP LOCKED`.

### Data Flow

**Task execution:** Goal → Planner → TaskGraph → Scheduler → Redis Stream → Worker → Execute → Complete → Unlock children

**State persistence:** State change → Postgres tx (UPDATE + INSERT event) → Redis Stream publish → Consumer updates cache

### Service Layout

Each service in `cmd/` is a standalone binary with REST/gRPC, health checks (`/health`, `/ready`), migrations on startup, and OpenTelemetry. Shared packages live in `pkg/` (db, config, logger, metrics, grpc, otel, health, storage, secrets, sdk).

## Engineering Standards (Hard Rules)

1. **PRD is law.** Never invent requirements not in `docs/PRD.md`. Cross-reference before implementing.
2. **Read before writing.** Always read relevant files before making changes.
3. **Clean architecture:** kernel → internal → cmd → pkg. Kernel = actors + tasks + scheduler + messaging + state.
4. **Production-ready code.** No TODOs, no prototypes. Include validation, error handling, structured logging.
5. **Consistency.** Maintain existing architecture patterns.

## Go Code Standards

- `context.Context` as first parameter on all I/O functions
- Structured logging only (`slog` or `zerolog`), never `fmt.Println` or bare `log.Println`
- Wrap errors: `fmt.Errorf("op: %w", err)`, never swallow errors
- Actor mailboxes: buffered channels, non-blocking sends (`select` with `default`)
- All hot-path reads from Redis/Memcached, **never** Postgres directly
- Table-driven tests for all exported functions
- After every change: `go vet ./...` && `golangci-lint run <changed_packages>` — fix before finishing
- No new gRPC endpoints or DB tables without PRD backing

## Tech Stack

Go 1.25, `pgx` (Postgres 17 + pgvector), `go-redis/v9` (Redis Streams), `google.golang.org/grpc`, `slog`/`zerolog` (logging), OpenTelemetry (tracing/metrics), `gomemcache` (Memcached), `golang-jwt/jwt/v5` (auth).

## Performance Targets (Blocking)

| Metric | Target |
|--------|--------|
| API read response (p99) | ≤ 10ms (from cache, never Postgres) |
| Task scheduling (median) | ≤ 50ms |
| Task scheduling (P95) | ≤ 500ms |
| Worker failure detection | ≤ 30s |
| Event persistence | ≤ 1s (async) |

**Anti-patterns (blocking):** `db.Query` in read handlers, synchronous LLM calls, unbounded `KEYS`/`SCAN`, missing connection pools, blocking mailbox sends.

## Security Policy (S1-S6) — Violations Are Blocking

- **S1 (mTLS):** All inter-service communication over mTLS
- **S2 (JWT):** All external APIs require JWT auth
- **S3 (RBAC/OPA):** All operations pass OPA policy checks via access-control service
- **S4 (Sandbox):** Tool executions in WASM/Docker/Firecracker with least privilege, resource limits, no unrestricted egress
- **S5 (Secrets):** No secrets in code, logs, or artifacts. Runtime injection from Vault only
- **S6 (Approval gates):** Dangerous ops (infra changes, prod deploys, data deletion) require human approval

## Proto & SQL Rules

- After `.proto` changes: `buf lint` && `buf generate` — fix failures
- Migrations must be idempotent (`IF NOT EXISTS`). Never drop columns without explicit user approval
- Use `FOR UPDATE SKIP LOCKED` for task claiming

## PRD Currency

Keep `docs/PRD.md` up to date when completing phases, shipping services, adding migrations, or changing API contracts. Update `scripts/validate.sh` when completing phases.

## Deployment

- **Local:** `./scripts/deploy.sh` — native-first (Postgres/Redis/Memcached), Docker for missing pieces
- **GCP:** `./scripts/gcp-deploy.sh` — GKE Autopilot, Cloud SQL, Memorystore, GCS (no MinIO on GCP)
- **CI:** GitHub Actions runs `go mod verify` → `go vet` → `golangci-lint` → `go test` → `go build` → `helm template`
