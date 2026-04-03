# Astra

A production-grade, microkernel-style operating system for autonomous agents. Built in Go with an actor-based architecture, Astra orchestrates persistent agents that plan, execute, collaborate, and learn.

**Target scale:** millions of agents, 100M+ tasks/day.

[![CI](https://github.com/prashanthrajagopal/astra/actions/workflows/ci.yml/badge.svg)](https://github.com/prashanthrajagopal/astra/actions/workflows/ci.yml)

---

## Features

- **Microkernel architecture** -- minimal kernel (actors, task graph, scheduler, message bus, state) with agent logic running outside via SDKs and APIs
- **16 services** -- control-plane, kernel, and worker tiers with clean separation of concerns
- **Task DAG engine** -- goals decomposed into dependency graphs, sharded across Redis Streams, claimed and executed by workers
- **Multi-tenancy** -- organizations, teams, users, RBAC, and agent visibility hierarchy (global/public/team/private)
- **Chat** -- WebSocket streaming, session management, and a built-in dashboard chat widget
- **Approvals** -- human-in-the-loop gates for plans and risky operations
- **LLM flexibility** -- pluggable providers (OpenAI, Ollama, MLX on Apple Silicon)
- **Slack integration** -- connect agents to Slack workspaces with OAuth and token rotation

## Architecture

```
+---------------------------------------------------------+
|  Applications (Agent Apps)                              |
|    +-- Astra SDK (agent dev framework)                  |
|          +-- Astra Kernel (microkernel)                 |
|                |- Actor Runtime                         |
|                |- Task Graph Engine                     |
|                |- Scheduler                             |
|                |- Message Bus (Redis Streams)           |
|                +- State Manager                        |
|---------------------------------------------------------|
|  Infrastructure                                         |
|    Postgres 17 (pgvector)  |  Redis 7  |  Memcached    |
+---------------------------------------------------------+
```

### Service tiers

| Tier | Services |
|------|----------|
| **Control-plane** | api-gateway, identity (JWT auth), access-control (RBAC/OPA) |
| **Kernel** | agent-service, goal-service, planner-service, scheduler-service, task-service, memory-service |
| **Workers** | execution-worker, browser-worker, worker-manager, tool-runtime, llm-router, prompt-manager, evaluation-service, cost-tracker |

### How it works

1. **Goal** -- user submits a goal via API or chat
2. **Plan** -- planner-service (LLM) produces a task DAG
3. **Schedule** -- scheduler detects ready tasks, pushes to Redis Streams (sharded)
4. **Execute** -- workers claim tasks via consumer groups, run them (LLM + tools)
5. **Complete** -- results flow back; child tasks are unblocked

```
User -> POST /goals -> goal-service -> planner (LLM) -> DAG
     -> scheduler -> Redis Streams -> execution-worker -> CompleteTask
     -> GET /tasks/{id} or /graphs/{id}
```

---

## Quick start

### Prerequisites

| Requirement | Purpose |
|-------------|---------|
| Go 1.25+ | Build and run all services |
| Postgres 17 | Primary database (pgvector) |
| Redis 7 | Messaging, ephemeral state, locks |
| Memcached | LLM/embedding/tool response cache |
| Docker (optional) | Starts Postgres/Redis/Memcached if not already running |

### Deploy locally

```bash
cp .env.example .env      # configure Postgres/Redis/Memcached
./scripts/deploy.sh        # builds all services, runs migrations, starts everything
./scripts/seed-agents.sh   # creates default agents (requires jq)
```

The deploy script uses existing Postgres/Redis/Memcached if running, otherwise starts them via Docker. Logs go to `logs/*.log`.

```bash
# Stop all services
for f in logs/*.pid; do kill $(cat $f) 2>/dev/null; done
```

### Deploy to GCP (GKE Autopilot)

```bash
cp scripts/.env.gcp.example .env.gcp
./scripts/gcp-deploy.sh --setup --dev   # first time: GKE, Cloud SQL, Memorystore, GCS
./scripts/gcp-deploy.sh --dev           # subsequent deploys
```

### Verify

- **Dashboard:** `http://localhost:8080/superadmin/dashboard/`
- **API:** `GET /agents` to list agents, `GET /tasks/{id}` for results
- **OpenAPI spec:** [docs/api/openapi.yaml](docs/api/openapi.yaml)
- **Postman collection:** [docs/api/Astra-Platform.postman_collection.json](docs/api/Astra-Platform.postman_collection.json)

---

## Development

### Build and test

```bash
# Build a single service
go build -o bin/SERVICE ./cmd/SERVICE

# Build all services
for svc in cmd/*/; do go build -o bin/$(basename $svc) ./$svc; done

# Run tests
go test ./... -count=1           # all tests
go test -short ./...             # unit only (skip integration)
go test ./internal/tasks -v     # single package

# Lint
go vet ./...
golangci-lint run ./internal/tasks/... ./cmd/task-service/...
```

### Project structure

```
cmd/                    # Service binaries (one per service)
internal/               # Core packages
  kernel/               # Actor registry (Spawn, Send, Stop)
  actors/               # Actor framework with buffered mailboxes
  messaging/            # Redis Streams with consumer groups
  scheduler/            # Task sharding and ready-task detection
  tasks/                # Task graph, state machine, DAG operations
  chat/                 # WebSocket chat sessions and messages
  rbac/                 # Role-based access control
pkg/                    # Shared libraries (db, config, logger, metrics, sdk)
migrations/             # Idempotent SQL migrations
deployments/helm/       # Helm charts for Kubernetes
scripts/                # Deploy, seed, and utility scripts
docs/                   # PRD, API specs, design docs
web/                    # Dashboard frontend (embedded in api-gateway)
```

### Tech stack

| Component | Technology |
|-----------|------------|
| Language | Go 1.25 |
| Database | Postgres 17 + pgvector (`pgx`) |
| Messaging | Redis Streams (`go-redis/v9`) |
| Cache | Memcached (`gomemcache`) |
| RPC | gRPC (`google.golang.org/grpc`) |
| Auth | JWT (`golang-jwt/jwt/v5`) |
| Logging | `slog` / `zerolog` |
| Observability | OpenTelemetry (tracing + metrics) |
| Deployment | Helm, GKE Autopilot, Docker |

### Performance targets

| Metric | Target |
|--------|--------|
| API read response (p99) | ≤ 10ms (from cache) |
| Task scheduling (median) | ≤ 50ms |
| Task scheduling (p95) | ≤ 500ms |
| Worker failure detection | ≤ 30s |

---

## LLM providers

### MLX (Apple Silicon)

On macOS with Apple Silicon, use [MLX-LM](https://github.com/ml-explore/mlx-lm) for local inference with Metal acceleration:

```bash
python -m pip install mlx-lm
mlx_lm.server --model mlx-community/Qwen2.5-7B-Instruct-4bit --port 8888
```

Configure in `.env`:

```bash
MLX_HOST=http://localhost:8888
MLX_MODEL=Qwen2.5-7B-Instruct-4bit
LLM_DEFAULT_PROVIDER=mlx
```

---

## Slack integration

Connect Astra agents to Slack workspaces. See [docs/slack-integration-design.md](docs/slack-integration-design.md) for full details.

1. Create a Slack app at [api.slack.com/apps](https://api.slack.com/apps)
2. Save credentials in **Super-Admin Dashboard > Slack**
3. Start the adapter: `go run ./cmd/slack-adapter`
4. Connect workspace via **Org Dashboard > Integrations > Connect Slack**
5. Invite the bot to a channel and send a message

---

## Documentation

- **[docs/PRD.md](docs/PRD.md)** -- product requirements document (source of truth)
- **[docs/api/openapi.yaml](docs/api/openapi.yaml)** -- OpenAPI spec
- **[docs/api/Astra-Platform.postman_collection.json](docs/api/Astra-Platform.postman_collection.json)** -- Postman collection

---

## License

See [LICENSE](LICENSE) for details.
