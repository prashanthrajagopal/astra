# Astra — Autonomous Agent Operating System

Astra is the **operating system for autonomous agents**: a production-grade, microkernel-style platform for persistent agents that plan, act, collaborate, remember, and learn. The kernel is minimal and high-performance; agent logic and applications run outside it via SDKs and APIs.

**Target scale:** Millions of agents, 100M+ tasks/day.

---

## Documentation

- **[docs/PRD.md](docs/PRD.md)** — Single source of truth: architecture, 16 canonical services, kernel APIs, database schema, Redis streams, deployment, security, and phased roadmap (v2.1).
- **API reference:** [docs/api/openapi.yaml](docs/api/openapi.yaml) — OpenAPI 3.x spec for all REST/HTTP APIs (agents, goals, tasks, graphs, identity, access-control, and internal services), with example flows; when adding or changing endpoints, update this spec and regenerate clients if used.
- **Design & deployment:** [docs/local-deployment.md](docs/local-deployment.md), [docs/deployment-design.md](docs/deployment-design.md), [docs/phase-history-usage-audit-design.md](docs/phase-history-usage-audit-design.md).
- **Approval system:** [docs/approval-system-extension-spec.md](docs/approval-system-extension-spec.md) — plan vs risky-task approvals, `AUTO_APPROVE_PLANS`, dashboard integration.
- **Chat agents:** [docs/chat-agents-design.md](docs/chat-agents-design.md) — WebSocket chat, streaming, sessions; [docs/chat-agents-implementation-plan.md](docs/chat-agents-implementation-plan.md) — implementation phases.
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

### How agents, planner, and workers work together

**What the agent actually does.** Agents do not execute tasks; workers do. The agent’s role is **identity, context, and ownership**:

- **Own the goal** — The goal is tied to an agent; the agent is who “asked for this work” and who the result belongs to.
- **Supply context** — The agent’s profile (system prompt, config, attached documents) is used when planning and when executing. The planner and workers act *on behalf of* that agent using this context.
- **Define “who”** — So the agent’s job is to be the identity and policy (persona, rules, constraints) that shapes what gets planned and how it is executed; workers perform the steps.

**How a specialist agent (e.g. “Python expert”) is applied.** The worker is generic; it does not “become” the expert. The agent’s expertise is **carried in the task payload**:

1. When a goal is created, **goal-service** assembles the agent’s context (system prompt, rules, skills) via `AssembleContext(agentID, goalID)` and passes it to the planner.
2. The **planner** embeds that context in **every task** it creates (e.g. `code_generate`, `shell_exec`) as `agent_context` in the task payload.
3. When the **execution-worker** runs a task, it reads `agent_context` from the payload, builds the prompt (system prompt + rules + task instructions), and sends it to the LLM. The LLM therefore behaves as the “Python expert” because that text is in the prompt. The worker is a generic executor; the expert is the context that travels with the task.

**Pending approvals (dashboard).** The dashboard lists two types of approval requests: (1) **Plan** — when `AUTO_APPROVE_PLANS` is false, creating a goal creates an approval request for the *implementation plan* before the task graph is created; approving it triggers goal-service to create the graph. (2) **Risky task** — when a worker tries to run a tool that policy marks as dangerous (e.g. `terraform apply`, certain `shell_exec`), the tool-runtime creates an approval request and waits; the dashboard lists these. The approvals table shows a **Type** column (plan / risky_task) and a detail modal with type-specific content. So approvals are human-in-the-loop for **implementation plans** (optional) and **risky tool runs**.

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
  Requires `jq`. The script is **idempotent** (skips creation if agents already exist). Agent **names are unique** (enforced by migration 0015); to de-duplicate existing data without adding the constraint, run `psql "$DATABASE_URL" -f scripts/dedup-agents-by-name.sql`. After deployment, run the seed once; use `GET /agents` to list agents and submit goals to any agent ID.

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

**Dashboard** (api-gateway `/dashboard/`): Summary stats include **Tokens In** and **Tokens Out** (from cost data). Goal detail modal shows actions; clicking a completed **code_generate** action opens a **Generated code** modal with path and content per file. Pending approvals show **Type** (plan / risky_task) and a type-specific detail modal.

### 3. Lifecycle

Goal → Plan (DAG) → Create tasks → Schedule (ready detection, Redis push) → Workers pull/claim/execute → CompleteTask/FailTask → Events and result APIs.

See [docs/PRD.md](docs/PRD.md) for the full specification.

---

## Architecture

The following diagrams align with [docs/PRD.md](docs/PRD.md). Each shows a different view of the system.

### High-level platform

Applications and SDK sit above the kernel; the kernel is minimal (actors, task graph, scheduler, message bus, state). Infrastructure is shared.

```
┌─────────────────────────────────────────────────────────┐
│  Applications (Agent Apps)                              │
│    └── Astra SDK (agent dev framework)                  │
│          └── Astra Kernel (microkernel)                 │
│                ├ Actor Runtime                          │
│                ├ Task Graph Engine                      │
│                ├ Scheduler                              │
│                ├ Message Bus                             │
│                └ State Manager                           │
├─────────────────────────────────────────────────────────┤
│  Infrastructure                                          │
│    Postgres (source of truth, pgvector)                 │
│    Redis (streams, state, locks)   Memcached (caches)   │
└─────────────────────────────────────────────────────────┘
```

### Kernel vs control-plane vs workers

**Control-plane:** API gateway, auth, and routing. **Kernel:** task graph, scheduling, and agent/task state. **Workers:** execution, tools, LLM, and evaluation.

```
┌──────────────────────────────────────────────────────────────────────┐
│  CONTROL-PLANE                                                        │
│  api-gateway │ identity │ access-control                              │
└────────────────────────────┬─────────────────────────────────────────┘
                              │ JWT + OPA
┌─────────────────────────────▼─────────────────────────────────────────┐
│  KERNEL                                                                │
│  agent-service │ goal-service │ planner-service │ scheduler-service   │
│  task-service │ memory-service                                        │
└─────────────────────────────┬─────────────────────────────────────────┘
                              │ Redis Streams (task shards)
┌─────────────────────────────▼─────────────────────────────────────────┐
│  WORKERS                                                               │
│  execution-worker │ browser-worker │ worker-manager │ tool-runtime     │
│  llm-router │ prompt-manager │ evaluation-service                     │
└──────────────────────────────────────────────────────────────────────┘
```

### Request-to-result data flow

End-to-end path from user request to task result: gateway → goals → planner → task-service → scheduler → Redis → workers → CompleteTask → APIs/events.

```
  User/API
      │
      ▼  POST /goals (JWT)
┌─────────────┐     ┌──────────────┐     ┌─────────────────┐
│ api-gateway │────▶│ goal-service │────▶│ planner-service │
└─────────────┘     └──────────────┘     └────────┬────────┘
       │                    │                    │ DAG
       │                    │                    ▼
       │                    │             ┌─────────────┐
       │                    │             │ task-service│ CreateGraph
       │                    │             └──────┬──────┘
       │                    │                    │
       │                    │                    ▼
       │                    │             ┌─────────────┐
       │                    │             │  scheduler   │ ready → queued
       │                    │             └──────┬──────┘
       │                    │                    │ XADD
       │                    │                    ▼
       │                    │             ┌─────────────┐
       │                    │             │ Redis Stream │ astra:tasks:shard:<n>
       │                    │             └──────┬──────┘
       │                    │                    │ XREADGROUP
       │                    │                    ▼
       │                    │             ┌─────────────┐     ┌──────────────┐
       │                    │             │ execution-  │────▶│ tool-runtime │
       │                    │             │ worker      │◀────│ llm-router   │
       │                    │             └──────┬──────┘     └──────────────┘
       │                    │                    │ CompleteTask / FailTask
       │                    │                    ▼
       │                    │             ┌─────────────┐     Postgres events
       │                    │             │ task-service│─────────────────────▶
       │                    │             └──────┬──────┘
       │                    │                    │
       ▼                    ▼                    ▼
  GET /tasks/{id}   GET /graphs/{id}   GET /agents/{id}/goals
```

### Service topology (16 canonical services + data stores)

All 16 services and their backing stores. Hot-path reads use Redis/Memcached; writes go to Postgres and emit to streams.

```
                    ┌─────────────┐
                    │ api-gateway │
                    └──────┬──────┘
         ┌────────────────┼────────────────┐
         ▼                ▼                 ▼
  ┌────────────┐   ┌────────────┐   ┌───────────────┐
  │  identity  │   │access-     │   │ agent-service │
  │            │   │control     │   │ goal-service  │
  └────────────┘   └────────────┘   │ planner-svc   │
                                    │ scheduler-svc │
                                    │ task-service  │
                                    │ memory-service│
                                    └───────┬───────┘
                                            │
    ┌───────────────────────────────────────┼───────────────────────────────────────┐
    ▼                   ▼                   ▼                   ▼                   ▼
┌────────┐         ┌────────┐         ┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│Postgres│         │ Redis  │         │ Memcached   │     │execution-    │     │ llm-router  │
│        │         │Streams │         │(LLM/embed/  │     │worker        │     │ prompt-mgr  │
│source  │         │state   │         │ tool cache) │     │browser-worker │     │ evaluation  │
│of truth│         │locks   │         └─────────────┘     │worker-manager │     │ tool-runtime│
└────────┘         └────────┘                             └──────────────┘     └─────────────┘
```

### Task lifecycle (state machine)

Tasks move through states; transitions are transactional and emit events. Workers drive scheduled → running → completed or failed.

```
  created ──▶ pending ──▶ queued ──▶ scheduled ──▶ running ──┬──▶ completed
       │         │          │           │            │      │
       │         │          │           │            └──────┴──▶ failed ──▶ (retry → queued | dead-letter)
       │         │          │           │
       │         │          │           └── Scheduler pushes to Redis; worker claims
       │         │          └── All deps completed; scheduler marks ready
       │         └── In graph, waiting for dependencies
       └── Task created in graph
```

### Agent–worker interaction

Agents (and kernel services) create and own tasks; they do not execute them. Workers pull from Redis, claim, execute, and report back.

```
  ┌─────────────────────────────────────────────────────────┐
  │  AGENT SIDE (create & own tasks)                         │
  │  agent-service, goal-service, planner-service,           │
  │  scheduler-service, task-service                         │
  │                                                          │
  │  CreateGoal → Plan (DAG) → CreateGraph → Mark ready       │
  │       │                                    │             │
  │       │                                    ▼             │
  │       │                            Push to Redis Stream  │
  └───────┼────────────────────────────────────┼─────────────┘
          │                                    │
          │                                    ▼
  ┌───────┼──────────────────────────────────────────────────┐
  │       │     Redis Streams (astra:tasks:shard:<n>)         │
  │       │     Consumer groups; workers XREADGROUP           │
  └───────┼────────────────────────────────────┼──────────────┘
          │                                    │
          │                                    ▼
  ┌───────┼──────────────────────────────────────────────────┐
  │  WORKER SIDE (execute tasks)                             │
  │  execution-worker, browser-worker                        │
  │  (use tool-runtime, llm-router, prompt-manager)          │
  │                                                          │
  │  Pull → Claim (lock, status scheduled→running)          │
  │    → Execute (LLM + tools) → CompleteTask / FailTask     │
  └──────────────────────────────────────────────────────────┘
```

### Deployment and network (Kubernetes view)

Typical namespace layout and flow: external traffic to api-gateway (mTLS between services); workers in dedicated namespace.

```
  ┌─────────────────────────────────────────────────────────────────────┐
  │  control-plane    api-gateway, identity, access-control              │
  │  (ingress / JWT)                                                     │
  └──────────────────────────────┬─────────────────────────────────────┘
                                  │ mTLS
  ┌───────────────────────────────▼─────────────────────────────────────┐
  │  kernel           scheduler-service, task-service, agent-service,    │
  │                   goal-service, planner-service, memory-service      │
  └───────────────────────────────┬─────────────────────────────────────┘
                                  │ Redis Streams / gRPC
  ┌───────────────────────────────▼─────────────────────────────────────┐
  │  workers          execution-worker, browser-worker, worker-manager, │
  │                   tool-runtime, llm-router, prompt-manager, eval-svc  │
  └───────────────────────────────┬─────────────────────────────────────┘
                                  │
  ┌───────────────────────────────▼─────────────────────────────────────┐
  │  infrastructure   Postgres, Redis, Memcached, MinIO/S3               │
  └─────────────────────────────────────────────────────────────────────┘
```

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

## MLX setup (Apple Silicon)

On macOS with Apple Silicon, you can use [MLX-LM](https://github.com/ml-explore/mlx-lm) for local LLM inference with Metal acceleration instead of (or as fallback after) Ollama.

1. **Install MLX-LM** (use the same Python you run from the shell):
   ```bash
   python -m pip install mlx-lm
   ```
   Verify: `python -c "import mlx; print('MLX installed')"`

2. **Start the MLX server** (in a separate terminal; keep it running):
   ```bash
   mlx_lm.server --model mlx-community/Qwen2.5-7B-Instruct-4bit --port 8888
   ```

3. **Configure `.env`**:
   ```bash
   MLX_HOST=http://localhost:8888
   MLX_MODEL=Qwen2.5-7B-Instruct-4bit
   LLM_DEFAULT_PROVIDER=mlx
   ```

4. **Deploy** — Run `./scripts/deploy.sh` as usual. On macOS, the deploy script may auto-detect MLX and set `LLM_DEFAULT_PROVIDER=mlx` if not already set.

**Other models:** Set `MLX_MODEL` to any MLX-compatible model (e.g. `mlx-community/Mistral-7B-Instruct-v0.3-4bit`). Astra talks to MLX via its OpenAI-compatible `/v1/chat/completions` endpoint. See [docs/local-deployment.md](docs/local-deployment.md) for more detail.

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
| `migrations/` | Idempotent SQL migrations (0001–0015; includes approval plan type, agents unique name) |
| `scripts/` | `deploy.sh`, `proto-generate.sh`, `create-python-expert-agent.sh`, `seed-agents.sh`, `dedup-agents-by-name.sql` |
| `deployments/` | Helm charts and K8s manifests |
| `docs/` | PRD, design docs (approval-system, chat-agents), phase history, runbooks |

---

## Recent changes

- **Approval system:** Two types — *plan* (implementation plan before creating task graph) and *risky_task* (dangerous tool run). `AUTO_APPROVE_PLANS` env; dashboard shows Type and detail modal. See [docs/approval-system-extension-spec.md](docs/approval-system-extension-spec.md).
- **Dashboard:** Tokens In/Out in summary; goal detail → click `code_generate` to view generated code; approvals table and modal with plan/risky_task.
- **Agents:** Unique names (migration 0015); idempotent seed; `scripts/dedup-agents-by-name.sql` for de-dup.
- **Chat agents:** Design and implementation plan for WebSocket chat with streaming ([docs/chat-agents-design.md](docs/chat-agents-design.md), [docs/chat-agents-implementation-plan.md](docs/chat-agents-implementation-plan.md)).

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
