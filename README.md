# Astra — Autonomous Agent Operating System

Astra is the **operating system for autonomous agents**: a production-grade, microkernel-style platform for persistent agents that plan, act, collaborate, remember, and learn. The kernel is minimal and high-performance; agent logic and applications run outside it via SDKs and APIs.

**Target scale:** Millions of agents, 100M+ tasks/day.

---

## Documentation

- **[docs/PRD.md](docs/PRD.md)** — Single source of truth: architecture, 16 canonical services, kernel APIs, database schema, Redis streams, deployment, security, and phased roadmap.
- **API reference:** [docs/api/openapi.yaml](docs/api/openapi.yaml) — OpenAPI 3.x spec for all REST/HTTP APIs (agents, goals, tasks, graphs, identity, access-control, and internal services), with example flows; when adding or changing endpoints, update this spec and regenerate clients if used.
- **Design & deployment:** [docs/local-deployment.md](docs/local-deployment.md), [docs/deployment-design.md](docs/deployment-design.md), [docs/phase-history-usage-audit-design.md](docs/phase-history-usage-audit-design.md).
- **Phase history:** [docs/phase-history/](docs/phase-history/) — what was built in each implementation phase (e.g. [phase-0.md](docs/phase-history/phase-0.md)).

---

## Agent vs worker

- **Agent** = logical entity that owns goals and task graphs. Agents (and kernel services) create tasks via the planner and task-service; they do **not** execute tasks.
- **Worker** = process that pulls from Redis task streams, claims and runs tasks, and reports `CompleteTask` / `FailTask`.

**Summary:** Agents (and kernel services) create and own tasks; workers execute them.

| | Nature | Creates tasks? | Executes tasks? | Canonical services |
|---|--------|----------------|------------------|--------------------|
| **Agent** | Logical entity (goals, DAGs) | Yes (planner, task-service) | No | agent-service, goal-service, planner-service, scheduler-service, task-service |
| **Worker** | Process (pulls from streams) | No | Yes | execution-worker, browser-worker, worker-manager, tool-runtime, llm-router, prompt-manager, evaluation-service |

---

## Example flow: coding agent

End-to-end when a user asks a coding agent to "write some code for me":

1. **User → goal** — `POST /goals` or `POST /agents/{id}/goals` with goal text.
2. **Goal → planner** — Goal-service calls planner-service (LLM); planner produces a DAG of tasks (e.g. `code_generate`, `shell_exec`) and task-service persists it via `CreateGraph`.
3. **Scheduler → Redis** — Scheduler marks ready tasks, pushes to Redis task stream; execution-worker pulls from the stream, claims and runs tasks (LLM + `file_write` via tool-runtime).
4. **CompleteTask → result** — Worker reports `CompleteTask`; user gets output via `GET /tasks/{id}` or `GET /graphs/{id}`.

```
User → POST /goals → goal-service → planner (LLM) → DAG → CreateGraph
       → scheduler → ready tasks → Redis stream → execution-worker
       → runs (LLM + tools) → CompleteTask → GET /tasks/{id} or /graphs/{id}
```

See [docs/PRD.md](docs/PRD.md) for full detail.

### Creating agents via API

After deployment, create specialized agents via the API Gateway and Identity service:

- **Python Expert** — An agent that only writes Python (3.10+, PEP 8, type hints, production-ready). Deploy using the script:
  ```bash
  ./scripts/create-python-expert-agent.sh
  ```
  The script obtains a JWT from Identity, creates the agent with `POST /agents` (actor_type `python-expert`), attaches a Python-only rule document, and prints the agent ID and an example `curl` to submit a goal. See [docs/api/openapi.yaml](docs/api/openapi.yaml) for the full request/response schema and other agent examples (e.g. e-commerce builder).

- **Seed multiple agents** — Create a default set of agents (Python Expert, Backend Dev, Frontend Dev, E-Commerce Builder, Generalist Coder, Documentation, DevOps, Testing) in one run:
  ```bash
  ./scripts/seed-agents.sh
  ```
  Requires `jq`. After deployment, run once to seed the platform; then use `GET /agents` to list agents and submit goals to any agent ID.

---

## How the platform works

This section gives new contributors and readers an end-to-end picture of Astra. **[docs/PRD.md](docs/PRD.md)** is the single source of truth for architecture, APIs, schema, and implementation details.

### 1. High-level flow

**From request to result:** A user or API client sends a request (e.g. create a goal) to the **api-gateway**. The gateway authenticates (JWT via identity) and authorizes (access-control/OPA), then routes to control-plane services. **Goals** are handled by **goal-service**, which invokes the **planner-service** (LLM) to produce a **task graph (DAG)**. The **task-service** persists the graph via `CreateGraph`. The **scheduler-service** periodically detects **ready** tasks (all dependencies completed), marks them queued, and **pushes** task messages to **Redis Streams** (sharded: `astra:tasks:shard:<n>`). **Workers** (e.g. **execution-worker**, **browser-worker**) pull from these streams via consumer groups, **claim** a task (lock, set status scheduled/running), **execute** it (LLM calls via **llm-router**, tool runs via **tool-runtime**), then report **CompleteTask** or **FailTask** to the **task-service**. Events are written to Postgres and/or published to Redis; the user gets results via **GET /tasks/{id}**, **GET /graphs/{id}**, or by subscribing to events.

In short: **Entry** (api-gateway, goals) → **Kernel** (actors, task graph, scheduler, message bus, state) → **Workers** (execution-worker, tool-runtime, llm-router) → **Results/events** back to the user. The [Example flow: coding agent](#example-flow-coding-agent) above is one concrete instance of this path.

### 2. Key components

**Kernel:** Actor runtime, task graph engine, scheduler, message bus, state.  
**Control-plane:** api-gateway, identity, access-control, agent-service, goal-service, planner-service, scheduler-service, task-service.  
**Worker-side:** execution-worker, browser-worker, worker-manager, tool-runtime, llm-router.  
**Data stores:** Postgres (source of truth), Redis (streams, state, locks), Memcached (LLM/embedding/tool caches).

### 3. Lifecycle

Goal → Plan (DAG) → Create tasks → Schedule (ready detection, Redis push) → Workers pull/claim/execute → CompleteTask/FailTask → Events and result APIs.

See [docs/PRD.md](docs/PRD.md) for the full specification.

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
   - Builds all service binaries, restarts all Astra services (stops by PID, then starts task-service, agent-service, scheduler-service, execution-worker, worker-manager, tool-runtime, browser-worker, memory-service, llm-router, prompt-manager, identity, access-control, planner-service, goal-service, evaluation-service, cost-tracker, api-gateway)
   - Logs: `logs/*.log`; PIDs: `logs/*.pid`  
   - Stop: `for f in logs/*.pid; do kill $(cat $f) 2>/dev/null; done`

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
| `scripts/` | `deploy.sh`, `proto-generate.sh`, `create-python-expert-agent.sh`, `seed-agents.sh` |
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
