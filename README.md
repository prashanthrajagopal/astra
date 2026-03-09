# Astra — Autonomous Agent Operating System

Astra is the **operating system for autonomous agents**: a production-grade, microkernel-style platform for persistent agents that plan, act, collaborate, remember, and learn. The kernel is minimal and high-performance; agent logic and applications run outside it via SDKs and APIs.

**Target scale:** Millions of agents, 100M+ tasks/day.

---

## Documentation

- **[docs/PRD.md](docs/PRD.md)** — Single source of truth: architecture, 16 canonical services, kernel APIs, database schema, Redis streams, deployment, security, and phased roadmap.
- **Design & deployment:** [docs/local-deployment.md](docs/local-deployment.md), [docs/deployment-design.md](docs/deployment-design.md), [docs/phase-history-usage-audit-design.md](docs/phase-history-usage-audit-design.md).
- **Phase history:** [docs/phase-history/](docs/phase-history/) — what was built in each implementation phase (e.g. [phase-0.md](docs/phase-history/phase-0.md)).

---

## Prerequisites

| Requirement | Purpose |
|-------------|---------|
| **Go 1.22+** | Build and run all services |
| **Postgres** (or Docker) | Primary DB; migrations in `migrations/` |
| **Redis** (or Docker) | Messaging (Redis Streams), ephemeral state, locks |
| **Memcached** (or Docker) | LLM/embedding/tool response cache |
| **psql** (if using native Postgres) | Running migrations when Postgres is not in Docker |
| **Docker** (optional) | Used only when Postgres/Redis/Memcached are not already running |

---

## Quick start (local dev)

1. **Clone and env**
   ```bash
   cp .env.example .env   # edit if needed (Postgres/Redis/Memcached host, user, password)
   ```

2. **Deploy (native-first)**
   ```bash
   ./scripts/deploy.sh
   ```
   The script:
   - Uses existing Postgres/Redis/Memcached if running, otherwise starts them via Docker
   - Runs all SQL migrations in `migrations/`
   - Builds `bin/api-gateway` and `bin/scheduler-service`, starts them in the background
   - Logs: `logs/api-gateway.log`, `logs/scheduler-service.log`  
   - Stop: `kill $(cat logs/api-gateway.pid) $(cat logs/scheduler-service.pid)`

3. **Build everything**
   ```bash
   go build ./...
   ```

See [docs/local-deployment.md](docs/local-deployment.md) for details. **Deployment** (including `scripts/deploy.sh`) is intended to be run by the DevOps agent or operator; see [.cursor/commands/deploy.md](.cursor/commands/deploy.md) and [.cursor/skills/devops-deployment/SKILL.md](.cursor/skills/devops-deployment/SKILL.md) for automation.

---

## Repo layout

| Path | Contents |
|------|----------|
| `cmd/` | Service entrypoints (api-gateway, scheduler-service, agent-service, task-service, execution-worker, etc.) |
| `internal/` | Kernel and service implementation (actors, kernel, tasks, scheduler, messaging, events, planner, etc.) |
| `pkg/` | Shared packages (db, config, logger, metrics, grpc, models, otel) |
| `proto/` | Protobuf/gRPC definitions; generated Go in `proto/kernel/`, `proto/tasks/` |
| `migrations/` | Idempotent SQL migrations (0001–0009) |
| `scripts/` | `deploy.sh`, `proto-generate.sh` |
| `deployments/` | Helm charts and K8s manifests |
| `docs/` | PRD, design docs, phase history, runbooks |

---

## Implementation and rules

- **Implementation follows [docs/PRD.md](docs/PRD.md).** All features, APIs, schema, and services are specified there; do not add behavior that isn’t in the PRD without updating it.
- **Cursor rules** in `.cursor/rules/` enforce PRD alignment, security (S1–S6), performance (10 ms reads, 50 ms scheduling), delegation, and PRD currency. Contributions should respect these rules.

---

## Proto codegen

```bash
./scripts/proto-generate.sh
```

See [docs/codegen.md](docs/codegen.md). CI runs `go vet`, `golangci-lint`, tests, and build; see [.github/workflows/ci.yml](.github/workflows/ci.yml).
