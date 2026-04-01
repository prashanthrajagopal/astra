# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Start local infrastructure (Postgres, Redis, Memcached)
docker-compose up -d

# Build all services
go build ./...

# Run all tests (with race detector)
go test ./... -race -count=1

# Run a single package's tests
go test ./internal/tasks/... -race -v

# Run a single test by name
go test ./internal/tasks/... -run TestTransition -v

# Lint changed packages
golangci-lint run ./internal/tasks/...

# Vet all packages
go vet ./...

# Regenerate protobuf (after editing .proto files)
./scripts/proto-generate.sh

# Deploy locally (DevOps only — starts infra, runs migrations, builds + starts all services)
./scripts/deploy.sh

# Validate all APIs against running services
./scripts/validate.sh

# Seed default agents after deploy
./scripts/seed-agents.sh

# Stop all running services
for f in logs/*.pid; do kill $(cat $f) 2>/dev/null; done
```

**After every Go change:** run `go vet ./...` and `golangci-lint run <changed_packages>` and fix all failures before finishing.

**After every `.proto` change:** run `buf lint` and `buf generate`.

**When completing a phase:** update PRD checkboxes in `docs/PRD.md` and add real validation checks to `scripts/validate.sh` (replace `skip_test` calls).

---

## Architecture

### Single source of truth

`docs/PRD.md` is the canonical specification for all architecture, APIs, database schema, Redis stream schemas, service descriptions, and the implementation roadmap. **Never add behavior that is not in the PRD without updating it first.** PRD updates must be in the same PR as the code change.

### 16 microservices across 3 layers

```
Control-plane:  api-gateway (8080, REST/WS)  identity (8085)  access-control (8086)
Kernel:         agent-service (9091, gRPC)   goal-service (8088)   planner-service (8087)
                scheduler-service            task-service (9090, gRPC)   memory-service (9092, gRPC)
Workers:        execution-worker   browser-worker   worker-manager (8082)
                tool-runtime (8083)   llm-router (9093, gRPC)   prompt-manager (8084)
                evaluation-service (8089)   cost-tracker (8090)
```

### Agents vs Workers — the key distinction

**Agents** are logical entities: they own goals and DAGs but never execute tasks. **Workers** are processes that pull from Redis Streams and execute tasks generically. Agent expertise travels as `agent_context` embedded in the task payload by goal-service (assembled from `system_prompt` + attached documents).

### Request-to-result flow

```
POST /goals → goal-service → planner-service (LLM → DAG) → task-service (CreateGraph)
           → scheduler (ready tasks → XADD astra:tasks:shard:<n>)
           → execution-worker (XREADGROUP → claim → LLM + tools via tool-runtime)
           → CompleteTask → GET /tasks/{id} or /graphs/{id}
```

### Package rules

- `cmd/<service>/` — entrypoints only; no business logic
- `internal/` — all private implementation; **kernel packages must not import service packages**
- `pkg/` — shared, stable libraries; **must not import `internal/`**
- No circular imports across `internal/` packages
- `pkg/sdk` — public SDK; must not import `internal/`

### Key internal packages

| Package | Responsibility |
|---------|---------------|
| `internal/actors` | Actor supervision framework (lifecycle, restarts) |
| `internal/agentdocs` | Agent profile + document assembly for `agent_context` |
| `internal/chat` | WebSocket chat sessions, streaming message delivery |
| `internal/codegen` | Language-aware code generation (detects language from task) |
| `internal/events` | Event sourcing — every state transition appends to `events` table |
| `internal/kernel` | Core kernel APIs (minimal, high-performance gRPC surface) |
| `internal/messaging` | Redis Streams consumers and producers |
| `internal/planner` | Goal → DAG planning via LLM |
| `internal/rbac` | Role-based access control engine |
| `internal/tasks` | Task graph model and state machine |

### Performance constraints (hard rules)

- **All API read endpoints ≤ 10ms (p99)** — serve from Redis/Memcached only, never synchronous Postgres
- **Scheduling latency ≤ 50ms median, ≤ 500ms P95**
- Write path: Postgres (source of truth) → emit to Redis Streams → async cache update
- LLM usage persisted asynchronously via `astra:usage` stream to keep hot path under 10ms
- Use `FOR UPDATE SKIP LOCKED` for task claiming to avoid lock contention

### Caching layers

| Store | Keys / Purpose |
|-------|---------------|
| Redis | `actor:state:<id>`, `lock:task:<id>` (Redlock), `worker:heartbeat:<id>`, `agent:profile:<id>`, `agent:docs:<id>`, `user:<id>`, task reads |
| Memcached | `llm:resp:{model}:{hash}` (24h), `embed:{hash}` (7–30d), `tool:cache:{tool}:{hash}` |

### Multi-tenancy model

Organizations → Teams → Users. JWT carries `user_id`, `org_id`, `org_role`, `team_ids`, `is_super_admin`. All data queries are scoped by `WHERE org_id = $orgID`. Agent visibility: `global` (platform) / `public` (org) / `team` / `private`. Super-admins see redacted metadata only — `redactForSuperAdmin()` strips `system_prompt`, `payload`, `result`, `goal_text`, code, and chat messages.

### Task state machine

```
created → pending → queued → scheduled → running → completed
                                                  → failed → (retry → queued | dead-letter)
```

Every transition is a single Postgres transaction that also inserts an event row (event sourcing). The `events` table is the immutable audit log.

### Approval system

Dangerous actions and plan approvals require human-in-the-loop. Two approval types exist: `plan` (DAG must be approved before execution) and `risky_task` (individual task pauses for review). Records stored in `approval_requests` table. Set `AUTO_APPROVE_PLANS=true` env var to bypass during development.

### Security constraints (S1–S6, all blocking)

- **S1**: All service-to-service communication uses mTLS (see `pkg/grpc`, `pkg/httpx`)
- **S2**: All external API calls require JWT from `identity` service
- **S3**: All operations pass OPA policy checks via `access-control` service
- **S4**: All tool executions run in WASM/Docker/Firecracker sandboxes with least privilege; secrets via ephemeral volumes only
- **S5**: Secrets injected from Vault at runtime; never in code, logs, or artifacts
- **S6**: Dangerous actions and plan approvals require human-in-the-loop (`approval_requests` table; `AUTO_APPROVE_PLANS` env var)

### Database migrations

Migrations live in `migrations/` and are numbered `0000`–`0018`. All migrations must be idempotent (`IF NOT EXISTS`, `IF EXISTS`). Never drop columns without explicit user approval.

### Redis Streams

| Stream | Purpose |
|--------|---------|
| `astra:tasks:shard:<n>` | Task dispatch; sharded by `hash(agent_id) % shard_count` |
| `astra:events` | Global event bus |
| `astra:worker:events` | Worker heartbeats (10s interval; >30s = re-queue) |
| `astra:usage` | Async LLM usage persistence |
| `astra:evaluation` | Evaluation results |

### Context propagation

`goal-service` assembles `AgentContext` from `agents.system_prompt` + `agent_documents` (rules sorted by priority, then skills, context_docs). This is serialized into every task payload as `agent_context`. `execution-worker` reads it and prepends to the LLM prompt. `internal/agentdocs` owns this assembly.

### LLM providers

Supports Ollama (local), OpenAI, Anthropic, Gemini, and MLX (Apple Silicon via `mlx_lm.server` on port 8888). Configured via `LLM_DEFAULT_PROVIDER`, `MLX_HOST`, `OLLAMA_HOST`, etc. LLM router in `internal/llm` handles model selection and Memcached caching.

### Observability

OpenTelemetry → OTLP → Jaeger/Tempo (tracing), Prometheus + Grafana (metrics in `deployments/grafana/`), structured JSON logging (`slog`). Dashboards: `/superadmin/dashboard/` (platform, redacted), `/org/dashboard` (org-admin, full), `/org/` (member home).


## Workflow Orchestration
#### 1. PLan Node Default
- Enter plan mode for ANY non-trivial task (3+ steps or architectural decisions)
- If something goes sideways, STOP and re-plan immediately - don't keep pushing
- Use plan mode for verification steps, not just building
- Write detailed specs upfront to reduce ambiguity
### 2. Subagent Strateg
- Use subagents liberally to keep main context window clea
- Offload research, exploration, and parallel analysis to subagents
- For complex problems, throw more compute at it via subagents
- One tack per subagent for focused execution ### 3. Self-Improvement Loop
After ANY correction from the user: update "tasks/lessons.md" with the pattern
- Write rules for yourself that prevent the same mistake
Ruthlessly iterate on these lessons until mistake rate drops
- Review lessons at session start for relevant project
### 4. Verification Before Done
- Never mark a task complete without proving it works
- Diff behavior between main and your changes when relevant
- Ask yourself: "Would a staff engineer approve this?"
- Run tests, check logs, demonstrate correctness ## 5. De and Elegance (Balanced)
- For non-trivial changes: pause and ask "is there a more elegant way?"
- If a fix feels hacky: "Knowing everything I know now, implement the elegant solution"
Skip this for simple, obvious fixes - don't over-engineer
- Challenge your own work before presenting it
### 6. Autonomous Bug Fizing
When given a bug report: just fix it. Don't ask for hand-holding Point at logs, errors, failing tests - then resolve them
- Zero context switching required from the user
- Go fix failing CI tests without being told how
## Task Management
**Plan First: Write plan to 'tasks/todo.md" with checkable items
2. **Verify Plan**: Check in before starting implementation
3. **Track Progress**: Mark items complete as you go
**Explain Changes**: High-level summary at each step
**Document Results**: Add review section to "tasks/todo.md
**Capture Lessons**: Update "tasks/lessons.md" after corrections
## Core Principles
**Simplicity First**: Make every change as simple as possible. Impact minimal code.
**No Laziness**: Find root causes. No temporary fixes. Senior developer standards.
- **Minimat Impact**: Changes should only touch what's necessary. Avoid introducing bugs.