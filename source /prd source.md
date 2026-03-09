
# ASTRA — The Autonomous Agent Operating System

## Final, Complete PRD & Engineering Specification
Version: 1.0 — Exportable Markdown

Purpose: A single, self‑contained specification that an engineering team can use to build Astra — a production‑grade, microkernel‑style, planet‑scale autonomous agent platform and SDK.

---

# Table of Contents

1. Overview & Vision
2. Core Capabilities & Non‑Goals
3. High‑level Architecture
4. Kernel (microkernel) design
5. Task Graph Engine
6. Actor Framework
7. Services & Microservice List
8. Monorepo layout
9. Go project layout
10. Message & Event Protocols
11. Database Schema
12. Caching
13. Tool runtime
14. Astra SDK
15. Agent taxonomy
16. Observability
17. Security
18. Deployment
19. Failure modes
20. CI/CD
21. Cost management
22. Operational playbooks
23. Acceptance criteria
24. Appendix

---

# 1. Overview & Vision

Vision: Astra is the operating system for autonomous agents.  
It runs persistent agents that plan, act, collaborate, remember and learn.

Target scale:

- millions of agents
- 100M+ tasks/day

Astra core is a minimal **microkernel**.

Everything else runs as services or SDK applications.

---

# 2. Core Capabilities

• Persistent agents  
• DAG task orchestration  
• Actor runtime  
• Redis message bus  
• Postgres durable state  
• Memcached fast cache  
• Tool sandbox runtime  
• Agent memory system  
• LLM routing and cost control  
• Observability & event sourcing

---

# 3. High‑level Architecture

Applications  
↓  
Astra SDK  
↓  
Astra Kernel  

Kernel Components:

- Actor Runtime
- Task Graph Engine
- Scheduler
- Message Bus
- State Manager

Infrastructure:

- Postgres
- Redis
- Memcached
- Object storage

---

# 4. Kernel Design

Kernel responsibilities:

1. Actor runtime
2. Task DAG engine
3. Scheduler
4. Message bus
5. State manager

Kernel API:

```
SpawnActor()
SendMessage()
CreateTask()
ScheduleTask()
CompleteTask()
FailTask()
QueryState()
SubscribeStream()
PublishEvent()
```

---

# 5. Task Graph Engine

Task graphs are DAGs.

States:

```
created → queued → scheduled → running → completed / failed
```

Ready task detection:

```sql
SELECT t.id
FROM tasks t
WHERE t.status='pending'
AND NOT EXISTS (
 SELECT 1 FROM task_dependencies d
 JOIN tasks td ON td.id=d.depends_on
 WHERE d.task_id=t.id
 AND td.status!='completed'
);
```

---

# 6. Actor Framework

Example Go actor interface:

```go
type Message struct {
  ID string
  Type string
  Source string
  Target string
  Payload json.RawMessage
}

type Actor interface {
  ID() string
  Receive(ctx context.Context, msg Message) error
  Stop() error
}
```

Actors run in goroutines.

Supervision tree:

```
SystemSupervisor
 └ AgentSupervisor
     ├ Planner
     ├ Memory
     └ Executor
```

---

# 7. Services (16)

1. api‑gateway
2. identity
3. access‑control
4. agent‑service
5. goal‑service
6. planner‑service
7. scheduler‑service
8. task‑service
9. llm‑router
10. prompt‑manager
11. evaluation‑service
12. worker‑manager
13. execution‑worker
14. browser‑worker
15. tool‑runtime
16. memory‑service

---

# 8. Monorepo Layout

```
/astra
  /cmd
  /internal
  /pkg
  /web
  /deployments
  /migrations
  /docs
  /tests
```

---

# 9. Go Project Layout

```
/internal/actors
/internal/agent
/internal/planner
/internal/scheduler
/internal/tasks
/internal/memory
/internal/workers
/internal/tools
/internal/evaluation
/internal/events
/internal/messaging
```

Shared packages:

```
/pkg/db
/pkg/config
/pkg/logger
/pkg/metrics
/pkg/grpc
/pkg/models
```

---

# 10. Redis Streams

```
astra:events
astra:tasks:shard:<n>
astra:agent:events
astra:worker:events
astra:evaluation
```

---

# 11. Database Schema

Example tables:

agents

```sql
CREATE TABLE agents (
 id UUID PRIMARY KEY,
 name TEXT,
 status TEXT,
 config JSONB
);
```

tasks

```sql
CREATE TABLE tasks (
 id UUID PRIMARY KEY,
 graph_id UUID,
 agent_id UUID,
 status TEXT,
 payload JSONB
);
```

---

# 12. Caching

Redis:

```
actor:state:<actor_id>
lock:task:<task_id>
```

Memcached:

```
llm:resp:{model}:{hash}
embed:{hash}
tool:cache:{tool}:{hash}
```

---

# 13. Tool Runtime

Sandbox types:

• WASM  
• Docker container  
• Firecracker microVM

Responsibilities:

- run tools safely
- enforce limits
- capture output
- return artifacts

---

# 14. Astra SDK

Example API:

```go
type AgentContext interface {
 ID() uuid.UUID
 Memory() MemoryClient
 CreateTask(t Task) error
 PublishEvent(ev Event) error
 CallTool(name string,input json.RawMessage)(ToolResult,error)
}
```

---

# 15. Agent Types

Examples:

- PRD parser
- planner
- backend dev
- frontend dev
- integration
- testing
- debugging
- devops
- learning

---

# 16. Observability

Tools:

- OpenTelemetry
- Prometheus
- Grafana
- Loki / Elastic

Metrics:

```
task_latency_seconds
task_success_rate
actor_count
worker_heartbeat_count
```

---

# 17. Security

• JWT auth  
• mTLS service comms  
• RBAC via OPA  
• secrets via vault  
• approval gates for dangerous tools

---

# 18. Deployment

Namespaces:

- control‑plane
- kernel
- workers
- infrastructure
- observability

Autoscaling via Kubernetes HPA.

---

# 19. Failure Handling

Worker crash:

```
task → requeue
worker restart
```

Postgres outage:

```
failover to replica
```

Redis outage:

```
replay events
```

---

# 20. CI/CD

Pipeline:

1. build
2. test
3. staging deploy
4. integration tests
5. canary deploy
6. production

---

# 21. Cost Control

LLM router chooses models:

- local model
- premium model

Caching and token quotas reduce cost.

---

# 22. Operations

On‑call rotation:

- kernel SRE
- agent platform

Incident lifecycle:

```
Detect → Triage → Contain → Fix → Postmortem
```

---

# 23. SLAs

API availability: **99.9%**

Scheduling latency:

- median ≤50ms
- p95 ≤500ms

---

# 24. Export

Save as:

```
astra_prd.md
```

Convert:

```
pandoc astra_prd.md -o astra_prd.pdf
```

---

# End of PRD
# ASTRA — The Autonomous Agent Operating System

**Final, Complete PRD & Engineering Specification**
Version: 1.0 — Exportable Markdown (ready for `pandoc` / GitHub / Notion)

> Purpose: a single, self-contained specification that an engineering team can use to build Astra — a production-grade, microkernel-style, planet-scale autonomous agent platform and SDK. This PRD contains architecture, microservices, monorepo layout, exact Go project layout, internal package boundaries, actor framework, task-graph kernel, DB schemas, Redis stream schemas, migration list, deployment, runbooks, testing and rollout plan, security, observability, cost controls, and developer workflows.

---

# Table of contents

1. Overview & Vision
2. Core Capabilities & Non-Goals
3. High-level Architecture (layers & responsibilities)
4. Kernel (microkernel) design — responsibilities & APIs
5. Task Graph Engine — model, lifecycle, algorithms
6. Actor Framework — interface, patterns, supervision
7. Services & Microservice List (16 canonical services)
8. Monorepo layout & repository rules
9. Exact Go project layout & internal package boundaries
10. Message & Event Protocols (actor message, Redis streams)
11. Database Schema, Indexes & Migrations (Postgres)
12. Caching & Fast-path (Redis + Memcached) schemas/keys
13. Tool runtime & sandboxing design
14. Astra SDK (agent API) — types, memory, tools, examples
15. Agent taxonomy (core agent types & workflows)
16. Observability, Tracing, Metrics, Dashboards
17. Security, Policy, Governance & Approval Flows
18. Deployment architecture, scaling, and capacity planning
19. Failure modes, recovery, and runbooks
20. CI/CD, testing, and release plan (phased roadmap)
21. Cost management & LLM routing / optimization strategies
22. Operational playbooks (oncall, incidents, upgrades)
23. Acceptance criteria & SLAs
24. Appendix: sample SQL, sample messages, export instructions

---

# 1. Overview & Vision

**Vision:** Astra is the operating system for autonomous agents. It runs persistent, long-lived agents that plan, act, collaborate, remember and learn. Astra core is a minimal, high-performance microkernel; everything else (agent logic, apps) runs outside the kernel via well-defined SDKs and APIs.

**Target outcome:** enable reliable autonomous workflows at planet-scale (millions of agents, 100M+ tasks/day) while keeping the platform maintainable, auditable and safe.

**Primary stakeholders:** engineering teams, ML/AI infra, platform SRE, product owners who want agents to deliver outcomes (e.g., autonomous developer app running on Astra).

---

# 2. Core Capabilities & Non-Goals

## Core Capabilities

* Persistent agent lifecycle management (spawn/stop/inspect)
* Task graph (DAG) planning, persistence and distributed execution
* Actor runtime (goroutine-based) with supervision
* Message bus (Redis Streams) for real-time coordination
* Durable state (Postgres) as single source of truth
* Fast caches (Memcached) and ephemeral state (Redis) for performance
* Tool runtime sandbox (container / microVM / WASM) for safe side-effects
* Memory: working (Redis), episodic/semantic (Postgres + pgvector)
* LLM router and cost-optimization features
* Observability, metrics, traces and event-sourcing
* Policy & governance: RBAC, approval flows, tool restrictions
* SDK for building agent applications in Go (and bindings for Python/TS)

## Non-Goals (explicit)

* Building full-featured LLMs — Astra integrates models via connectors
* Replacing specialized data platforms (but integrate them)
* Tight coupling between application logic and kernel — app logic remains outside kernel

---

# 3. High-level Architecture (layers & responsibilities)

```
Applications (Agent apps)
  └── Astra SDK (agent dev framework)
        └── Astra Kernel (microkernel)
              ├ Actor Runtime
              ├ Task Graph Engine
              ├ Scheduler
              ├ Message Bus
              └ State Manager (Postgres)
Infrastructure
  ├ Postgres (primary store + replicas)
  ├ Redis (streams, ephemeral state)
  ├ Memcached (LLM/embedding cache)
  └ Object Storage (MinIO / S3)
```

**Key idea:** kernel + SDK + applications (strict separation). Kernel exposes Actor, Task, Messaging and State APIs. SDK implements agent developer primitives.

---

# 4. Kernel (microkernel) design — responsibilities & APIs

## Kernel responsibilities (minimal)

1. **Actor Runtime** — run millions of actors efficiently (goroutine-per-actor, mailbox model).
2. **Task Graph Engine** — persist & coordinate DAGs, dependency resolution, partial executions.
3. **Scheduler** — shard-aware distributed scheduler, capability matching, priority.
4. **Message Bus** — Redis Streams + local in-memory mailboxes.
5. **State Manager** — transactional persistence in Postgres, event sourcing, snapshots.

## Kernel invariants

* Kernel must be small and stable.
* All non-kernel services run user-space (SDK/services).
* Kernel guarantees: message delivery semantics within configured SLAs, consistent task state, and transactionally consistent state writes.

## Kernel API (exposed RPC / gRPC)

* `SpawnActor(actor_spec)` → ActorID
* `SendMessage(actorID, Message)`
* `CreateTask(task_spec)` → TaskID
* `ScheduleTask(taskID)`
* `CompleteTask(taskID, result)`
* `FailTask(taskID, error)`
* `QueryState(entity, filters)`
* `SubscribeStream(stream_name, consumer_group)`
* `PublishEvent(event)`

APIs are implemented as gRPC endpoints and well-documented protobuf messages.

---

# 5. Task Graph Engine — model, lifecycle, algorithms

## Task Graph model

* `TaskGraph` = DAG of `TaskNodes`.
* Each `TaskNode` = id, type, agent_id, payload (JSONB), status, priority, retries, metadata.

## Task lifecycle

* States: `created` → `queued` → `scheduled` → `running` → `completed` / `failed` → `dead-letter`.
* Transitions persist in Postgres; change events appended to `events` table (event sourcing).

## Scheduling algorithm (high-level)

1. Detect ready nodes (dependencies satisfied). Query Postgres using `NOT EXISTS` dependency check or materialized ready queue.
2. Mark ready nodes atomically and push to Redis stream shard `astra:tasks:shard_n`.
3. Workers pull from consumer groups. Consumers claim and `SET task.status = scheduled` (transaction).
4. Worker executes → `TaskStarted` event, heartbeat; on completion `CompleteTask` writes result, emits `TaskCompleted`.
5. On completion, scheduler unlocks children; they become ready.

## Sharding

* Shard by `agent_id` or `task_graph_id` using consistent hashing. Scheduler instances coordinate shards (each scheduler can own a shard subset). Shard assignment stored in Postgres.

## Fault tolerance

* If worker heartbeat lost → claim timeout → scheduler marks task as `queued` and re-pushes.
* Dead-letter queue for tasks exceeding retry threshold.
* Event replay: events table reconstructs graph state.

## Partial graph execution & long-running tasks

* Worker heartbeats & periodic update of long-running tasks; timeouts enforced; partial requeueing possible.

---

# 6. Actor Framework — interface, patterns, supervision

## Actor primitives

* `Actor` = {ID, mailbox (chan Message), state (opaque), behavior (Receive function)}.
* Actors run as goroutines on kernel nodes. Actors can be colocated or distributed.

## Core actor interface (Go)

```go
type Message struct {
  ID        string
  Type      string
  Source    string
  Target    string
  Payload   json.RawMessage
  Meta      map[string]string
  Timestamp time.Time
}

type Actor interface {
  ID() string
  Receive(ctx context.Context, msg Message) error
  Stop() error
}
```

## Base actor impl

`BaseActor` provides mailbox, lifecycle, metrics hooks, supervised restart policy.

## Supervision

* Supervisor actors watch children; policies: restart (immediate/backoff), escalate, terminate.
* Supervisor tree: SystemSupervisor → AgentSupervisor(s) → SubActors (Planner, Memory, Executor).
* Supervisor restarts limited by circuit breaker to avoid “restart storms”.

## Actor communication

* Local: direct in-process channels (low latency).
* Cross-node: publish/subscribe via Redis Streams; kernel maps actor location; `SendMessage` proxies between nodes.

## Actor persistence

* Actors that manage durable state (e.g., AgentActor) persist snapshots to Postgres periodically; on restart, state restored.

---

# 7. Services & Microservice List

**Canonical services (16)** — implement as independent processes in monorepo; each can scale horizontally:

1. `api-gateway` — Auth, REST/gRPC endpoints, versioning.
2. `identity` — User/service auth, tokens, audit log.
3. `access-control` — RBAC, OPA policy enforcement.
4. `agent-service` — Agent lifecycle, Actor supervisor integration.
5. `goal-service` — Goal ingestion, validation, routing.
6. `planner-service` — Core planner that turns goals → TaskGraphs.
7. `scheduler-service` — Distributed scheduler & shard manager.
8. `task-service` — Task CRUD, dependency engine API.
9. `llm-router` — Model routing, caching, rate-limiting connector.
10. `prompt-manager` — Prompt templates, versions, A/B prompts.
11. `evaluation-service` — Result validators, test harnesses, auto-evaluators.
12. `worker-manager` — Worker registration, health, scaling hints.
13. `execution-worker` — General worker runtime (executes tasks).
14. `browser-worker` — Headless browser automation worker.
15. `tool-runtime` — Tool sandboxes (WASM/container/firecracker controllers).
16. `memory-service` — Episodic and semantic memory, embedding pipelines.

**Notes:** `worker-manager` + `execution-worker` represent a pool; multiple specialized worker types exist as subservices.

---

# 8. Monorepo layout & repository rules

```
/astra
  /cmd                 # service entrypoints (one folder per service)
    /api-gateway
    /identity
    /access-control
    ...
  /internal            # implementation-only packages (not exported)
    /actors
    /agent
    /planner
    /scheduler
    /tasks
    /memory
    /workers
    /tools
    /evaluation
    /events
    /messaging
  /pkg                 # shared packages intended for public reuse across services
    /db
    /config
    /logger
    /metrics
    /grpc
    /models
  /web                 # frontend React app
  /deployments         # k8s manifests, helm charts, infra scripts
  /migrations          # SQL migrations
  /scripts             # dev utilities
  /docs                # PRD, design docs, runbooks
  /tests               # integration/e2e fixtures
```

**Repo rules**

* `internal/*` packages are not importable outside monorepo services (enforced by `go vet` / linter).
* `pkg/*` are stable, versioned, documented packages.
* CI enforces `go vet`, `golangci-lint`, `staticcheck`. PRs require tests and changelog.

---

# 9. Exact Go project layout & internal package boundaries

Top-level packages and responsibilities:

```
/cmd/<service>
/internal/actors         # kernel actor runtime implementation
/internal/agent          # agent lifecycle, AgentActor
/internal/planner        # planner orchestration, plan validators
/internal/scheduler      # scheduling & shard manager
/internal/tasks          # task model, transitions, persistence logic
/internal/memory         # memory APIs, embedding pipeline
/internal/workers        # worker orchestration, heartbeats
/internal/tools          # tool runtime control (sandboxing API)
/internal/evaluation     # evaluators, test harness integration
/internal/events         # event store, event replay
/internal/messaging      # Redis stream clients & helpers
/pkg/db                  # db conn, migrations, helpers
/pkg/config              # config loader (env / vault)
/pkg/logger              # structured logging
/pkg/otel                # traces exporter
```

**Responsibilities by package**

* `actors` = kernel runtime primitives (mailboxes, actor lifecycle, supervision)
* `agent` = agent-level orchestration only (calls kernel actors)
* `scheduler` = shard ownership, ready set detection
* `tasks` = task state machine, API to transition tasks safely (transactions)
* `memory` = read/write memory, search embeddings, cache keys
* `tools` = sandbox lifecycle, tool permission checks
* `messaging` = unified Redis Streams helper API for consumer groups, backoff, ack

---

# 10. Message & Event Protocols

## Actor Message (JSON / protobuf)

```json
{
  "id":"uuid",
  "type":"TaskStarted",
  "source":"worker-123",
  "target":"agent-abc",
  "payload": {"task_id":"...", "meta": {...}},
  "timestamp":"2026-03-XXT..."
}
```

## Core message types

* `GoalCreated`, `PlanRequested`, `PlanGenerated`
* `TaskCreated`, `TaskScheduled`, `TaskStarted`, `TaskCompleted`, `TaskFailed`
* `MemoryWrite`, `MemoryRead`
* `ToolExecutionRequested`, `ToolExecutionCompleted`
* `EvaluationRequested`, `EvaluationCompleted`

## Redis Streams (names & fields)

**1.** `astra:events` — global event stream
Fields: `event_id, type, actor_id, payload, timestamp`

**2.** `astra:tasks:shard:<n>` — shard-specific task queue
Fields: `task_id, graph_id, agent_id, task_type, payload, priority, created_at`

**3.** `astra:agent:events`
Fields: `agent_id, event_type, payload, timestamp`

**4.** `astra:worker:events`
Fields: `worker_id, event_type, task_id, metadata, timestamp`

**5.** `astra:evaluation`
Fields: `task_id, evaluator_id, result, metadata, timestamp`

Consumer groups used for each shard; messages are acknowledged when processed.

---

# 11. Database Schema, Indexes & Migrations (Postgres)

## Key tables (DDL summaries)

### `agents`

```sql
CREATE TABLE agents (
  id UUID PRIMARY KEY,
  name TEXT,
  status TEXT,
  config JSONB,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT now(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT now()
);
```

### `goals`

```sql
CREATE TABLE goals (
  id UUID PRIMARY KEY,
  agent_id UUID REFERENCES agents(id),
  goal_text TEXT,
  priority INT DEFAULT 100,
  status TEXT,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT now()
);
CREATE INDEX idx_goals_agent ON goals(agent_id);
```

### `tasks`

```sql
CREATE TABLE tasks (
  id UUID PRIMARY KEY,
  graph_id UUID,
  goal_id UUID REFERENCES goals(id),
  agent_id UUID,
  type TEXT,
  status TEXT,
  payload JSONB,
  result JSONB,
  priority INT DEFAULT 100,
  retries INT DEFAULT 0,
  max_retries INT DEFAULT 5,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT now(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT now()
);
CREATE INDEX idx_tasks_agent ON tasks(agent_id);
CREATE INDEX idx_tasks_status ON tasks(status);
CREATE INDEX idx_tasks_graph ON tasks(graph_id);
```

### `task_dependencies`

```sql
CREATE TABLE task_dependencies (
  task_id UUID REFERENCES tasks(id),
  depends_on UUID REFERENCES tasks(id),
  PRIMARY KEY (task_id, depends_on)
);
CREATE INDEX idx_task_dep_dependson ON task_dependencies(depends_on);
```

### `memories`

```sql
CREATE TABLE memories (
  id UUID PRIMARY KEY,
  agent_id UUID REFERENCES agents(id),
  memory_type TEXT,
  content TEXT,
  embedding VECTOR(1536),
  created_at TIMESTAMP WITH TIME ZONE DEFAULT now()
);
CREATE INDEX idx_memories_agent ON memories(agent_id);
-- vector index
CREATE INDEX idx_memory_embedding ON memories USING ivfflat (embedding);
```

### `artifacts`

```sql
CREATE TABLE artifacts (
  id UUID PRIMARY KEY,
  agent_id UUID,
  task_id UUID,
  uri TEXT,
  metadata JSONB,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT now()
);
```

### `events`

```sql
CREATE TABLE events (
  id BIGSERIAL PRIMARY KEY,
  event_type TEXT,
  actor_id UUID,
  payload JSONB,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT now()
);
CREATE INDEX idx_events_actor ON events(actor_id);
```

## Migrations list (suggested)

```
0001_initial_schema.sql
0002_task_dependencies.sql
0003_memories_embedding_pgvector.sql
0004_artifacts.sql
0005_workers_table.sql
0006_indexes.sql
0007_event_table.sql
0008_task_status_constraints.sql
```

(Migrations include constraints, FK relations, and necessary trigger functions for updated_at.)

---

# 12. Caching & Fast-path (Redis + Memcached) schemas/keys

## Redis usage

* Streams as specified above.
* Short-lived actor state: `actor:state:<actor_id>` (hash) with TTL for working memory.
* Distributed locks: `lock:task:<task_id>` using Redlock pattern.

## Memcached keys (examples)

* `llm:resp:{model}:{prompt_hash}` → cached model responses, TTL based.
* `embed:{content_hash}` → embedding vector (serialized).
* `tool:cache:{tool_name}:{input_hash}` → tool execution results.

Cache eviction and TTL policies are enforced to ensure fresh data: LLM responses TTL ~ 24h; embedding caches longer (7–30d) depending on retention policy.

---

# 13. Tool runtime & sandboxing design

## Purpose

Tools are the mechanism through which agents take side effects: git operations, container runs, API calls, infra changes.

## Sandbox types

* **WASM runtime** for safe lightweight plugins.
* **Container sandbox** (Docker) for common tools that require filesystem and processes.
* **MicroVM (Firecracker)** for riskier workloads (e.g., executing untrusted code, build tasks).

## Tool runtime service responsibilities

* Launch sandbox with least-privileged role and isolated network (egress controls).
* Enforce resource limits (CPU, mem, disk, time).
* Provision secrets via ephemeral volumes (no plaintext logs).
* Capture stdout/stderr and return as artifacts.
* Policy engine check before tool execution (access control).

## Tool permissioning

* Tool capabilities are flagged, and agents require scopes; access controlled by `access-control` service (OPA-based).

---

# 14. Astra SDK (agent API) — types, memory, tools, examples

## Goal: Make building agents straightforward, safe and auditable.

### Core Go SDK primitives

```go
type Goal struct { ID uuid.UUID; Text string; Priority int }
type Task struct { ID uuid.UUID; Type string; Payload json.RawMessage }
type AgentContext interface {
  ID() uuid.UUID
  Memory() MemoryClient
  CreateTask(t Task) error
  PublishEvent(ev Event) error
  CallTool(name string, input json.RawMessage) (ToolResult, error)
}
```

### Memory API

* `WriteMemory(agentID, type, content, embedding?)`
* `SearchMemory(agentID, query, topK)`
* `GetMemoryByID(id)`

### Tool API

* `ExecuteTool(ctx, toolName, input, options) -> ToolResult`
* Tool results stored as artifacts and event.

### Example agent skeleton

```go
type SimpleAgent struct { id uuid.UUID }
func (a *SimpleAgent) Plan(ctx AgentContext, goal Goal) []Task { ... }
func (a *SimpleAgent) Execute(ctx AgentContext, t Task) (Result, error) { ... }
func (a *SimpleAgent) Reflect(ctx AgentContext, outcome Outcome) { ... }
```

SDK contains utilities for prompt templating, embeddings, LLM caching, and metrics instrumentation.

---

# 15. Agent taxonomy (core agent types & workflows)

**Core agent categories** (examples with responsibilities):

* **PRD-Parser Agent** — parse structured requirements.
* **Planner Agent** — generate Task Graphs.
* **Backend Dev Agent** — create API endpoints, generate code, run unit tests.
* **Frontend Dev Agent** — scaffold React components and pages.
* **Integration Agent** — wire services and test E2E.
* **Testing Agent** — generate tests, run CI.
* **Debugging Agent** — triage failing tests and propose fixes.
* **DevOps Agent** — generate infra config, deploy pipelines.
* **Learning Agent** — optimize prompts and strategy based on feedback.

**Agent workflow example** (build from PRD):

1. PRD-Parser extracts features.
2. Planner builds DAG.
3. Backend/Frontend agents create code tasks.
4. Worker pool executes tasks; evaluation service validates results.
5. Debugging agents fix issues, re-run tasks.
6. DevOps agent deploys on successful pipeline.

---

# 16. Observability, Tracing, Metrics, Dashboards

## Tracing & logs

* OpenTelemetry for distributed traces (span per task execution, per tool call).
* Centralized logs (structured JSON) forwarded to logging cluster (Elastic / Loki).

## Metrics

* Prometheus metrics: task_latency_seconds, task_success_rate, events_processed_total, actor_count, worker_heartbeat_count, llm_token_usage.
* Alerts for high failure rate, high task queue depth, low worker availability.

## Dashboards

* **Cluster Overview**: capacity, active agents, task throughput, error rate.
* **Agent Health**: per-agent throughput, avg latency.
* **Cost**: LLM token usage & cost per agent.
* **Task Graph Viewer**: interactive graph per goal to see node states.

## Tracing details

* Each `Task` creates a root span; tool calls are child spans with resource attributes (tool name, CPU time).
* Use `trace_id` to tie logs ↔ traces ↔ events.

---

# 17. Security, Policy, Governance & Approval Flows

## Authentication & Identity

* Identity service handles users, service accounts.
* JWT + mTLS for service-to-service.

## Authorization

* Access-control service uses OPA policies with RBAC; policies stored in Git and versioned.
* Agents run with minimum privileges; per-agent permission scopes.

## Secrets

* Secrets injected at runtime from Vault-like system; no secret persisted in logs or artifacts.

## Tool & action governance

* Dangerous actions (delete infra, change prod) require approval gates (human-in-the-loop).
* `policy-engine` intercepts tool execution events and allows/denies based on policy.

## Audit & compliance

* Immutable event log (events table) + artifact immutability for forensic auditing.

---

# 18. Deployment architecture, scaling, and capacity planning

## K8s cluster layout (recommended)

* `control-plane` namespace: api-gateway, identity, access-control
* `kernel` namespace: scheduler, task-service, actors
* `workers` namespace: execution-worker, specialized workers
* `infrastructure` namespace: redis, memcached (clustered), postgres (primary + replicas), minio
* `observability` namespace: prometheus, grafana, opentelemetry-collector

## Scaling model

* Stateless services → autoscale horizontally (HPA based on CPU / queue depth).
* Workers → autoscale by Redis queue length and scheduler hints.
* Redis → cluster mode with shard count based on throughput.
* Postgres → primary for writes + read replicas for heavy reads (materialized views).

## Capacity planning (starting point)

* Node types: control (c4.large), worker (c8.large with GPUs for inference), DB (r5), cache (memory-optimized).
* Estimate throughput per worker (tasks/hour) and derive worker count.

---

# 19. Failure modes, recovery, and runbooks

## Key failure modes

* Worker crashes: task requeue, notify owner, attempt automatic fix via `worker-manager`.
* Postgres outage: make read-only mode, fail-safe circuit for short window; promote replica if needed.
* Redis failure: failover to replica, replay events from `events` table if gaps detected.
* Task graph corruption: reconstruct via event-sourcing replay, quarantine corrupted graphs.

## Runbook snippets

* **Worker lost**: Identify last heartbeat → check `worker_events` stream → move in-flight tasks back to `astra:dead_tasks` or re-queue → restart worker node.
* **High error rate**: examine `task_failed` rates → sample task traces → if widespread, pause new goal intake (API gateway toggles) → roll back last deploys.
* **LLM cost spike**: disable large-model routing (llm-router admin toggle) → enforce lower model tiers → alert finance.

---

# 20. CI/CD, testing, and release plan (phased roadmap)

## CI rules

* PRs must pass: unit tests, linters, integration tests (service bindings), contract tests for protobuf APIs.
* Service-level test harnesses under `/tests`.

## Staging pipeline

1. Build and test images.
2. Deploy to staging cluster.
3. Run full integration + simulated agent workloads (test DAGs).
4. Canary deploy to production with 5% traffic, monitor metrics for 30 minutes, then full rollout.

## Release cadence

* Weekly minor deploys, feature-gated.
* Quarterly major releases (Schema migrations planned with versioned migration tool).

---

# 21. Cost management & LLM routing / optimization strategies

## LLM routing

* `llm-router` chooses between local models (cheap) and premium (expensive) based on task classification, priority and budget.
* Routing rules example: `classification -> local model`, `high-risk reasoning -> premium model`, `code generation -> code-specialized model`.

## Cost controls

* Token quota per agent / per org.
* Burst budget with hard caps and administrative approval.
* Caching responses (Memcached) for repeated prompts.
* Batch inference where possible (GPU worker).

## Monitoring cost

* `cost_tracking` service monitors token usage, model calls per agent, cost per task and raises alerts if thresholds exceeded.

---

# 22. Operational playbooks (oncall, incidents, upgrades)

## Oncall rotations

* Primary oncall for kernel (SRE)
* Secondary oncall for agent ops (platform)
* Escalation matrix documented in `docs/runbooks`

## Incident lifecycle

* Detect → Triage → Contain → Remediate → Postmortem → Remediation review.

## Upgrade plan

* Kernel upgrades are backward compatible (message contracts, schemas).
* Rolling upgrade with canary and DB migration windows; use blue/green where possible.

---

# 23. Acceptance criteria & SLAs

## Functional acceptance (MVP)

* Spawn and run persistent agent (simple echo agent).
* Planner produces task DAGs and scheduler executes tasks end-to-end.
* Worker executes tasks and returns results; tasks persisted in Postgres.
* Observability traces visible for each task.
* Tool runtime can run sandboxed command and return artifact.

## Production SLAs

* Control plane API: 99.9% availability.
* Task execution latency (scheduling): ≤ 50ms median for ready tasks; ≤ 500ms P95 for most interactive tasks.
* Task execution correctness: pass rate ≥ 99% (non-environmental tasks).
* Time to detect worker failure: ≤ 30s.
* Event durability: events persisted within 1s to Postgres.

---

# 24. Appendix: sample SQL, sample messages, export instructions

## Sample SQL (task ready detection)

```sql
-- find pending tasks whose deps are completed
SELECT t.id
FROM tasks t
WHERE t.status = 'pending'
  AND NOT EXISTS (
    SELECT 1 FROM task_dependencies d
    JOIN tasks td ON td.id = d.depends_on
    WHERE d.task_id = t.id AND td.status != 'completed'
  )
FOR UPDATE SKIP LOCKED;
```

## Sample migration file name list

```
0001_initial_schema.sql
0002_task_dependencies.sql
0003_memories_embedding_pgvector.sql
0004_artifacts_workers.sql
0005_events_indexing.sql
0006_task_status_constraints.sql
```

## Sample Redis message (JSON)

```json
{
  "id":"uuid",
  "type":"TaskScheduled",
  "source":"scheduler-01",
  "target":"worker-pool",
  "payload": {"task_id":"...", "graph_id":"...", "payload": {...}},
  "timestamp":"2026-03-XXT..."
}
```

## Example actor mailbox pattern (Go)

```go
mailbox := make(chan Message, 1024)
actor := BaseActor{ID: id, Mailbox: mailbox}
go actor.Run()
```

## Exporting this PRD

* Save as `astra_prd.md` and convert:
  `pandoc astra_prd.md -o astra_prd.pdf`
* Or upload to GitHub repo `/docs/PRD.md` or to Notion/Confluence directly.

---

# Implementation Roadmap (milestones & timeline)

**Phase 0 — Prep (2 weeks)**

* Finalize infra accounts, Postgres + Redis + Memcached + MinIO deployment.
* Repo scaffolding.

**Phase 1 — Kernel MVP (8–10 weeks)**

* Implement Actor Runtime, State Manager (Postgres), basic Message Bus (Redis Streams), Task Graph Engine minimal features, scheduling loop, `api-gateway` and `agent-service`.
* Unit tests, integration harness.

**Phase 2 — Workers & Tool Runtime (6–8 weeks)**

* `execution-worker`, `worker-manager`, `tool-runtime` sandboxes, sample browser worker.

**Phase 3 — Memory & LLM routing (6 weeks)**

* `memory-service`, embedding pipeline (pgvector), `llm-router`, Memcached caching.

**Phase 4 — Orchestration, eval, security (6–8 weeks)**

* `planner-service`, `evaluation-service`, OPA integration, policy and approval gates.

**Phase 5 — Scale & production hardening (8 weeks)**

* Scaling tests (load to baseline throughput), observability dashboards, runbooks, cost tracking, SLO enforcement.

**Phase 6 — SDK & Applications (ongoing)**

* Build Astra SDK, sample autonomous developer app, sample research agent.

---

# Final notes & next steps

* This PRD is intentionally prescriptive: same architecture used by modern infra systems (microkernel concept, Postgres as source-of-truth, Redis Streams for real-time messaging, memcached for hot cache).
* Next immediate artifacts to generate (I can produce if you want right away):

  1. Full SQL migration files (the 8 files listed).
  2. Starter Go skeleton for kernel + actor runtime (first ~2,000 lines) with tests.
  3. Protobuf/gRPC schemas for kernel APIs.
  4. Minimal helm chart for staging deployment.

If you want any of those generated now, say which artifact to produce first and I’ll emit it as code or files in the chat.
