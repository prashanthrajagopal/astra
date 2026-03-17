# ASTRA — The Autonomous Agent Operating System

**Engineering Specification v3.0 — Single-Platform**

> A single, self-contained specification for building Astra — a production-grade, microkernel-style, planet-scale autonomous agent platform and SDK. Contains architecture, Go interfaces, gRPC contracts, database DDL, Redis stream schemas, deployment manifests, and phased implementation roadmap.

---

# Table of Contents

1. [Overview & Vision](#1-overview--vision)
2. [Core Capabilities & Non-Goals](#2-core-capabilities--non-goals)
3. [High-level Architecture](#3-high-level-architecture)
4. [Monorepo Layout & Go Package Boundaries](#4-monorepo-layout--go-package-boundaries)
5. [Kernel Design — Responsibilities & APIs](#5-kernel-design--responsibilities--apis)
6. [Actor Framework — Interface, Patterns, Supervision](#6-actor-framework--interface-patterns-supervision)
7. [Task Graph Engine — Model, Lifecycle, Algorithms](#7-task-graph-engine--model-lifecycle-algorithms)
8. [Scheduler — Sharding, Ready Detection, Dispatch](#8-scheduler--sharding-ready-detection-dispatch)
9. [Services — 16 Canonical Microservices](#9-services--16-canonical-microservices)
10. [Protobuf & gRPC Contracts](#10-protobuf--grpc-contracts)
11. [Database Schema, Indexes & Migrations](#11-database-schema-indexes--migrations)
12. [Message & Event Protocols](#12-message--event-protocols)
13. [Caching & Fast-path — Redis + Memcached](#13-caching--fast-path--redis--memcached)
14. [Tool Runtime & Sandboxing](#14-tool-runtime--sandboxing)
15. [Astra SDK — Agent API](#15-astra-sdk--agent-api)
16. [Agent Taxonomy & Workflows](#16-agent-taxonomy--workflows)
17. [Observability, Tracing, Metrics](#17-observability-tracing-metrics)
18. [Security, Policy, Governance](#18-security-policy-governance)
19. [Multi-Tenancy Architecture](#19-multi-tenancy-architecture)
20. [Deployment Architecture & Scaling](#20-deployment-architecture--scaling) (includes [Platform & Hardware Acceleration](#platform--hardware-acceleration))
21. [Failure Modes, Recovery & Runbooks](#21-failure-modes-recovery--runbooks)
22. [CI/CD, Testing & Release Plan](#22-cicd-testing--release-plan)
23. [Cost Management & LLM Routing](#23-cost-management--llm-routing)
24. [Operational Playbooks](#24-operational-playbooks)
25. [Acceptance Criteria & SLAs](#25-acceptance-criteria--slas)
26. [Implementation Roadmap](#26-implementation-roadmap)
27. [Build Order & Dependency Graph](#27-build-order--dependency-graph)

---

# 1. Overview & Vision

**Vision:** Astra is the operating system for autonomous agents. It runs persistent, long-lived agents that plan, act, collaborate, remember and learn. Astra core is a minimal, high-performance microkernel; everything else (agent logic, apps) runs outside the kernel via well-defined SDKs and APIs.

**Target scale:** Millions of agents, 100M+ tasks/day.

**Primary stakeholders:** Engineering teams, ML/AI infra, platform SRE, product owners building autonomous agent applications on Astra.

**Hard performance constraint:** No API call may take more than 10ms to respond. All hot-path reads serve from cache (Redis/Memcached), never synchronous Postgres.

### Document maintenance (PRD currency)

The PRD (`docs/PRD.md`) is the **single source of truth** for Astra architecture, schema, roadmap, and canonical services. It must be kept current:

- When features, schema, APIs, or implementation phases change, the PRD is updated **in the same work** (same PR or same phase completion). Do not ship changes that alter architecture, APIs, or schema without updating the corresponding PRD sections.
- **Tech Lead** is responsible for ensuring PRD updates when a phase is completed or when architectural/schema decisions are made during implementation.
- **Architect** reviews and approves material PRD changes (new sections, contract changes, or structural edits).

### Recent changes (v2.1)

- **Approval system:** Two types — *plan* (implementation plan approval before creating task graph) and *risky_task* (dangerous tool execution). `AUTO_APPROVE_PLANS` env; goal-service returns 202 and creates plan approval when disabled; access-control calls goal-service `apply-plan` on plan approval. See **docs/approval-system-extension-spec.md**.
- **Dashboard:** Goal detail modal lists actions; clicking a completed `code_generate` action opens a "Generated code" modal (path + content per file). Summary stats include **Tokens In** and **Tokens Out** (from cost data). Approvals table has **Type** column (plan / risky_task); approval detail modal shows type-specific content.
- **Agents:** Agent names are unique; migration 0015 de-duplicates by name and adds `UNIQUE(agents.name)`. Standalone script `scripts/dedup-agents-by-name.sql` for de-dup without constraint. Seed script is idempotent and resilient to gateway startup.
- **Codegen:** Task result for `code_generate` includes `generated_files` (path + content) for dashboard display.
- **Chat agents (design):** WebSocket-based chat agents with streaming, sessions, and optional tool/worker calls. Design: **docs/chat-agents-design.md**; implementation plan: **docs/chat-agents-implementation-plan.md**.
- **Slack integration (design):** Connect Astra chat agents to Slack; one workspace → one org; slack-adapter service + Redis queue + worker; OAuth and Vault for tokens. Platform Slack app secrets (Signing Secret, Client ID, Client Secret, Redirect URL) configurable from super-admin UI (stored encrypted); env fallback. Design: **docs/slack-integration-design.md**.

---

# 2. Core Capabilities & Non-Goals

## Core Capabilities

- Persistent agent lifecycle management (spawn/stop/inspect)
- Task graph (DAG) planning, persistence and distributed execution
- Actor runtime (goroutine-based) with supervision trees
- Message bus (Redis Streams) for real-time coordination
- Durable state (Postgres) as single source of truth
- Fast caches (Memcached) and ephemeral state (Redis) for performance
- Tool runtime sandbox (WASM / Docker container / Firecracker microVM) for safe side-effects
- Memory: working (Redis), episodic/semantic (Postgres + pgvector)
- LLM router and cost-optimization features
- Observability, metrics, traces and event-sourcing
- Policy & governance: RBAC, approval flows, tool restrictions
- SDK for building agent applications in Go (bindings for Python/TS later)
- **Platform-aware hardware acceleration:** On macOS, leverage Metal, Neural Engine, and GPU where beneficial for inference, embeddings, and compute; on Linux, support standard CPU and CUDA-based deployments. Astra can be deployed in production on both macOS (e.g. Mac Mini, Mac Studio) and Linux (e.g. Kubernetes, cloud, on-prem).
- **Real-time chat agents** via WebSocket with streaming responses, tool invocation, and session management.

## Non-Goals

- Building LLMs — Astra integrates models via connectors
- Replacing specialized data platforms (but integrates them)
- Tight coupling between application logic and kernel — app logic remains outside kernel

---

# 3. High-level Architecture

```
┌─────────────────────────────────────────────┐
│          Applications (Agent Apps)           │
│  └── Astra SDK (agent dev framework)        │
│        └── Astra Kernel (microkernel)       │
│              ├ Actor Runtime                │
│              ├ Task Graph Engine            │
│              ├ Scheduler                    │
│              ├ Message Bus                  │
│              └ State Manager               │
├─────────────────────────────────────────────┤
│              Infrastructure                  │
│  ├ Postgres (primary + replicas, pgvector)  │
│  ├ Redis (streams, ephemeral state, locks)  │
│  ├ Memcached (LLM/embedding/tool cache)     │
│  └ Object Storage (MinIO / S3)              │
└─────────────────────────────────────────────┘
```

**Key principle:** Kernel + SDK + Applications (strict separation). Kernel exposes Actor, Task, Messaging and State APIs. SDK implements agent developer primitives. Applications are built on the SDK.

---

# 4. Monorepo Layout & Go Package Boundaries

```
/astra
  /cmd                         # service entrypoints (one folder per service)
    /api-gateway/main.go
    /identity/main.go
    /access-control/main.go
    /agent-service/main.go
    /goal-service/main.go
    /planner-service/main.go
    /scheduler-service/main.go
    /task-service/main.go
    /llm-router/main.go
    /prompt-manager/main.go
    /evaluation-service/main.go
    /worker-manager/main.go
    /execution-worker/main.go
    /browser-worker/main.go
    /tool-runtime/main.go
    /memory-service/main.go
  /internal                    # private implementation packages
    /actors                    # kernel: actor runtime, BaseActor, mailbox, supervision
    /agent                     # agent lifecycle orchestration, AgentActor
    /kernel                    # kernel manager: actor registry, message dispatch
    /planner                   # planner orchestration, plan validators
    /scheduler                 # scheduling loop, shard management, ready-task detection
    /tasks                     # task model, state machine, DAG, transitions, persistence
    /memory                    # memory APIs, embedding pipeline, pgvector search
    /workers                   # worker orchestration, heartbeats, health checks
    /tools                     # tool runtime control, sandbox lifecycle, permission checks
    /agentdocs                 # agent document store, profile/context assembly
    /evaluation                # evaluators, test harness integration
    /events                    # event store, event replay, event sourcing
    /messaging                 # Redis Streams clients, consumer groups, backoff, ack
    /llm                       # LLM router logic, model selection, caching
    /identity                  # user CRUD, login/auth, password hashing
    /rbac                      # role-based access control, middleware
  /pkg                         # shared packages (stable, documented, importable)
    /db                        # DB connection pool, migration runner, helpers
    /config                    # config loader (env vars, Vault)
    /logger                    # structured logging (slog/zerolog)
    /metrics                   # Prometheus metrics registration
    /grpc                      # gRPC server/client helpers, interceptors, mTLS
    /models                    # shared domain types (UUID, Status enums)
    /otel                      # OpenTelemetry traces exporter
  /proto                       # protobuf/gRPC definitions
    kernel.proto
    task.proto
  /migrations                  # SQL migration files (ordered, idempotent)
  /deployments                 # Helm charts, k8s manifests, infra scripts
    /helm/astra/
  /cmd/api-gateway
    /superadmin/               # platform dashboard (HTML/CSS/JS)
    /login/                    # login page (HTML/CSS/JS)
  /web                         # frontend (future)
  /scripts                     # dev utilities
  /docs                        # this PRD, architecture docs, runbooks
  /tests                       # integration/e2e test fixtures
  go.mod
  go.sum
  Dockerfile
  docker-compose.yml
```

## Package Responsibilities

| Package | Layer | Responsibility |
|---|---|---|
| `internal/actors` | Kernel | Actor runtime primitives: `Actor` interface, `BaseActor`, mailbox, lifecycle, supervision tree |
| `internal/kernel` | Kernel | Kernel manager: actor registry, `Spawn()`, `Send()`, message routing |
| `internal/tasks` | Kernel | Task model, `Graph`, `Status` enum, state machine transitions (transactional) |
| `internal/scheduler` | Kernel | Shard ownership, ready-set detection, Redis stream dispatch, heartbeat monitoring |
| `internal/messaging` | Kernel | Unified Redis Streams helper: consumer groups, publish, subscribe, ack, backoff |
| `internal/events` | Kernel | Event store (Postgres), event replay, event sourcing |
| `internal/agent` | Service | Agent-level orchestration, `AgentActor` (calls kernel actors) |
| `internal/planner` | Service | Planner orchestration, goal → DAG conversion, plan validators |
| `internal/memory` | Service | Memory read/write, embedding pipeline, pgvector search, cache keys |
| `internal/workers` | Service | Worker pool management, heartbeats, task assignment, health checks |
| `internal/tools` | Service | Sandbox lifecycle (WASM/Docker/Firecracker), tool permission checks |
| `internal/evaluation` | Service | Evaluators, result validators, test harness integration |
| `internal/agentdocs` | Service | Agent document store (rules, skills, context docs), profile/context assembly for planner and workers |
| `internal/llm` | Service | LLM model routing, cost-based selection, response caching |
| `internal/identity` | Service | User CRUD, login/authentication, password hashing (bcrypt), JWT enrichment |
| `internal/rbac` | Service | Role-based authorization engine, super-admin checks, HTTP/gRPC middleware |
| `pkg/db` | Shared | Postgres connection pool (`pgx`), migration runner |
| `pkg/config` | Shared | Config loader (environment variables, Vault integration) |
| `pkg/logger` | Shared | Structured logging setup (`slog` or `zerolog`) |
| `pkg/metrics` | Shared | Prometheus metrics registration and helpers |
| `pkg/grpc` | Shared | gRPC server/client helpers, interceptors (auth, tracing, logging) |
| `pkg/models` | Shared | Shared domain types used across packages |
| `pkg/otel` | Shared | OpenTelemetry exporter configuration |

## Repository Rules

- `internal/*` packages are not importable outside the monorepo (enforced by Go compiler).
- `pkg/*` are stable, versioned, documented packages.
- CI enforces `go vet`, `golangci-lint`, `staticcheck`. PRs require tests and changelog.
- No circular imports between `internal/` packages; kernel packages must not import service packages.

---

# 5. Kernel Design — Responsibilities & APIs

## Kernel Responsibilities (minimal set)

1. **Actor Runtime** — run millions of actors efficiently (goroutine-per-actor, mailbox model)
2. **Task Graph Engine** — persist & coordinate DAGs, dependency resolution, partial executions
3. **Scheduler** — shard-aware distributed scheduler, capability matching, priority
4. **Message Bus** — Redis Streams + local in-memory mailboxes
5. **State Manager** — transactional persistence in Postgres, event sourcing, snapshots

## Kernel Invariants

- Kernel must be small and stable.
- All non-kernel services run in user-space (SDK/services).
- Kernel guarantees: message delivery within configured SLAs, consistent task state, transactionally consistent state writes.

## Kernel API (gRPC)

| RPC | Input | Output | Description |
|---|---|---|---|
| `SpawnActor` | `actor_spec` | `ActorID` | Create and start a new actor goroutine |
| `SendMessage` | `actorID, Message` | `ack` | Deliver message to actor mailbox |
| `CreateTask` | `task_spec` | `TaskID` | Create task node in a graph |
| `ScheduleTask` | `taskID` | `ack` | Mark task ready, push to worker queue |
| `CompleteTask` | `taskID, result` | `ack` | Mark task completed, store result, unlock children |
| `FailTask` | `taskID, error` | `ack` | Mark task failed, increment retries or dead-letter |
| `QueryState` | `entity, filters` | `state` | Query entity state (agents, tasks, workers) |
| `SubscribeStream` | `stream, consumer_group` | `stream Event` | Subscribe to Redis stream |
| `PublishEvent` | `event` | `ack` | Publish event to a stream |

## Reference Implementation — Kernel Manager

```go
package kernel

import (
    "context"
    "fmt"
    "sync"

    "astra/internal/actors"
)

type Kernel struct {
    mu     sync.RWMutex
    actors map[string]actors.Actor
}

func New() *Kernel {
    return &Kernel{
        actors: make(map[string]actors.Actor),
    }
}

func (k *Kernel) Spawn(a actors.Actor) {
    k.mu.Lock()
    defer k.mu.Unlock()
    k.actors[a.ID()] = a
}

func (k *Kernel) Send(ctx context.Context, target string, msg actors.Message) error {
    k.mu.RLock()
    a, ok := k.actors[target]
    k.mu.RUnlock()
    if !ok {
        return fmt.Errorf("kernel.Send: actor %s not found", target)
    }
    return a.Receive(ctx, msg)
}
```

---

# 6. Actor Framework — Interface, Patterns, Supervision

## Core Types

```go
package actors

import (
    "context"
    "encoding/json"
    "sync"
    "time"
)

type Message struct {
    ID        string            `json:"id"`
    Type      string            `json:"type"`
    Source    string            `json:"source"`
    Target    string            `json:"target"`
    Payload   json.RawMessage   `json:"payload"`
    Meta      map[string]string `json:"meta"`
    Timestamp time.Time         `json:"timestamp"`
}

type Actor interface {
    ID() string
    Receive(ctx context.Context, msg Message) error
    Stop() error
}
```

## BaseActor — Reference Implementation

```go
var ErrMailboxFull = fmt.Errorf("actor mailbox full")

type BaseActor struct {
    id      string
    mailbox chan Message
    stop    chan struct{}
    wg      sync.WaitGroup
}

func NewBaseActor(id string) *BaseActor {
    return &BaseActor{
        id:      id,
        mailbox: make(chan Message, 1024),
        stop:    make(chan struct{}),
    }
}

func (a *BaseActor) ID() string { return a.id }

func (a *BaseActor) Start(handler func(context.Context, Message) error) {
    a.wg.Add(1)
    go func() {
        defer a.wg.Done()
        for {
            select {
            case msg := <-a.mailbox:
                if err := handler(context.Background(), msg); err != nil {
                    slog.Error("actor handler error", "actor", a.id, "err", err)
                }
            case <-a.stop:
                return
            }
        }
    }()
}

// Non-blocking send — never blocks the caller
func (a *BaseActor) Receive(ctx context.Context, msg Message) error {
    select {
    case a.mailbox <- msg:
        return nil
    default:
        return ErrMailboxFull
    }
}

func (a *BaseActor) Stop() error {
    close(a.stop)
    a.wg.Wait()
    return nil
}
```

## Supervision Tree

```
SystemSupervisor
 └ AgentSupervisor(s)
     ├ PlannerActor
     ├ MemoryActor
     └ ExecutorActor
```

### Supervision Policies

| Policy | Behavior |
|---|---|
| `RestartImmediate` | Restart child immediately on failure |
| `RestartBackoff` | Restart with exponential backoff (100ms → 200ms → ... → 30s cap) |
| `Escalate` | Propagate failure to parent supervisor |
| `Terminate` | Stop child permanently |

Circuit breaker: limit restarts to `maxRestarts` within a time window to prevent restart storms.

## Actor Communication

- **Local**: Direct in-process channel send (low latency, ~1μs)
- **Cross-node**: Publish via Redis Streams → kernel maps actor location → `SendMessage` proxies between nodes

## Actor Persistence

- Actors managing durable state (e.g., `AgentActor`) snapshot to Postgres periodically
- On restart, state restored from latest snapshot
- Snapshots stored as JSONB in the `events` table or dedicated snapshot table

---

# 7. Task Graph Engine — Model, Lifecycle, Algorithms

## Task Graph Model

- `TaskGraph` = DAG of `TaskNode`s
- Each `TaskNode` = id, type, agent_id, graph_id, payload (JSONB), status, priority, retries, max_retries, result (JSONB)

## Go Types

```go
package tasks

type Status string

const (
    StatusCreated   Status = "created"
    StatusPending   Status = "pending"
    StatusQueued    Status = "queued"
    StatusScheduled Status = "scheduled"
    StatusRunning   Status = "running"
    StatusCompleted Status = "completed"
    StatusFailed    Status = "failed"
)

type Task struct {
    ID         string
    GraphID    string
    GoalID     string
    AgentID    string
    Type       string
    Status     Status
    Payload    map[string]interface{}
    Result     map[string]interface{}
    Priority   int
    Retries    int
    MaxRetries int
}

type Dependency struct {
    TaskID    string
    DependsOn string
}

type Graph struct {
    ID           string
    Tasks        []Task
    Dependencies []Dependency
}
```

## Task Lifecycle

```
created → pending → queued → scheduled → running → completed
                                                  → failed → (retry → queued | dead-letter)
```

Every transition persists to Postgres in a transaction and appends to the `events` table (event sourcing).

## Valid State Transitions

| From | To | Trigger |
|---|---|---|
| `created` | `pending` | Task added to graph, waiting for dependencies |
| `pending` | `queued` | All dependencies completed, task is ready |
| `queued` | `scheduled` | Scheduler assigned task to a worker |
| `scheduled` | `running` | Worker started execution |
| `running` | `completed` | Worker finished successfully |
| `running` | `failed` | Worker reported error |
| `failed` | `queued` | Retry (if retries < max_retries) |
| `failed` | `dead-letter` | Exhausted retries |

## Transactional State Transition

```go
func (s *Store) Transition(ctx context.Context, taskID string, from, to Status, payload json.RawMessage) error {
    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil {
        return fmt.Errorf("tasks.Transition: begin: %w", err)
    }
    defer tx.Rollback()

    res, err := tx.ExecContext(ctx,
        `UPDATE tasks SET status = $1, updated_at = now() WHERE id = $2 AND status = $3`,
        to, taskID, from)
    if err != nil {
        return fmt.Errorf("tasks.Transition: update: %w", err)
    }
    if rows, _ := res.RowsAffected(); rows == 0 {
        return ErrInvalidTransition
    }

    _, err = tx.ExecContext(ctx,
        `INSERT INTO events (event_type, actor_id, payload, created_at) VALUES ($1, $2, $3, now())`,
        "Task"+string(to), taskID, payload)
    if err != nil {
        return fmt.Errorf("tasks.Transition: event: %w", err)
    }

    return tx.Commit()
}
```

---

# 8. Scheduler — Sharding, Ready Detection, Dispatch

## Ready Task Detection

```sql
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

## Scheduling Algorithm

1. **Detect ready nodes** — query Postgres with the above SQL (uses `FOR UPDATE SKIP LOCKED` to avoid lock contention between scheduler instances)
2. **Mark ready atomically** — `UPDATE tasks SET status = 'queued'` in same transaction
3. **Push to Redis stream** — `XADD astra:tasks:shard:<n>` with task metadata
4. **Workers pull** — consumer groups on the shard stream, claim and `SET status = 'scheduled'`
5. **Worker executes** — emit `TaskStarted` event, periodic heartbeat
6. **On completion** — `CompleteTask` writes result, emits `TaskCompleted`, scheduler unlocks children

## Sharding

- Shard key: `hash(agent_id) % shard_count` or `hash(graph_id) % shard_count`
- Each scheduler instance owns a subset of shards (assignment stored in Postgres)
- Consistent hashing for rebalancing when scheduler instances scale

## Reference Implementation

```go
package scheduler

import (
    "context"
    "database/sql"
    "time"

    "astra/internal/messaging"
    "astra/pkg/logger"
)

type Scheduler struct {
    db  *sql.DB
    bus *messaging.Bus
}

func New(db *sql.DB, bus *messaging.Bus) *Scheduler {
    return &Scheduler{db: db, bus: bus}
}

func (s *Scheduler) Run(ctx context.Context) error {
    ticker := time.NewTicker(100 * time.Millisecond)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            if err := s.tick(ctx); err != nil {
                logger.Error("scheduler tick", "err", err)
            }
        }
    }
}

func (s *Scheduler) tick(ctx context.Context) error {
    ready, err := s.findReadyTasks(ctx)
    if err != nil {
        return err
    }
    for _, taskID := range ready {
        if err := s.enqueue(ctx, taskID); err != nil {
            logger.Error("enqueue failed", "task", taskID, "err", err)
        }
    }
    return nil
}

func (s *Scheduler) findReadyTasks(ctx context.Context) ([]string, error) {
    rows, err := s.db.QueryContext(ctx, `
        SELECT t.id FROM tasks t
        WHERE t.status = 'pending'
        AND NOT EXISTS (
            SELECT 1 FROM task_dependencies d
            JOIN tasks td ON td.id = d.depends_on
            WHERE d.task_id = t.id AND td.status != 'completed'
        )
        FOR UPDATE SKIP LOCKED
        LIMIT 100`)
    if err != nil {
        return nil, fmt.Errorf("scheduler.findReady: %w", err)
    }
    defer rows.Close()
    var ids []string
    for rows.Next() {
        var id string
        if err := rows.Scan(&id); err != nil {
            return nil, err
        }
        ids = append(ids, id)
    }
    return ids, rows.Err()
}

func (s *Scheduler) enqueue(ctx context.Context, taskID string) error {
    _, err := s.db.ExecContext(ctx,
        `UPDATE tasks SET status = 'queued', updated_at = now() WHERE id = $1 AND status = 'pending'`,
        taskID)
    if err != nil {
        return err
    }
    return s.bus.Publish(ctx, "astra:tasks:shard:0", map[string]interface{}{
        "task_id": taskID,
    })
}
```

## Worker Heartbeat & Failure Detection

- Workers send heartbeats every 10s to `astra:worker:events` stream
- If heartbeat not received within 30s, scheduler marks in-flight tasks as `queued` and re-pushes
- Tasks exceeding `max_retries` move to dead-letter queue

---

# 9. Services — 16 Canonical Microservices

Each service runs as an independent process in the monorepo (`cmd/<service>/main.go`). Each can scale horizontally.

| # | Service | Responsibility | Namespace |
|---|---|---|---|
| 1 | `api-gateway` | REST/gRPC gateway, auth middleware, rate limiting, versioning; chat sessions, WebSocket streaming | control-plane |
| 2 | `identity` | User management (CRUD, login, password reset), JWT tokens (user_id, email, is_super_admin, scopes), service-to-service tokens | control-plane |
| 3 | `access-control` | Policy engine (super_admin, approval workflows), approval assignment and actions | control-plane |
| 4 | `agent-service` | Agent lifecycle (spawn/stop/inspect), actor supervisor integration, agent profile & document management | kernel |
| 5 | `goal-service` | Goal ingestion, validation, routing to planner, context assembly (system_prompt + documents) | kernel |
| 6 | `planner-service` | Core planner: goals → TaskGraphs using LLM | kernel |
| 7 | `scheduler-service` | Distributed scheduler, shard manager, ready-task dispatch | kernel |
| 8 | `task-service` | Task CRUD, dependency engine API, state queries | kernel |
| 9 | `llm-router` | Model routing (local/premium), response caching, rate limiting | workers |
| 10 | `prompt-manager` | Prompt templates, versions, A/B prompt experiments | workers |
| 11 | `evaluation-service` | Result validators, test harnesses, auto-evaluators | workers |
| 12 | `worker-manager` | Worker registration, health monitoring, scaling hints | workers |
| 13 | `execution-worker` | General worker runtime (executes tasks from Redis streams) | workers |
| 14 | `browser-worker` | Headless browser automation worker (Playwright/Puppeteer) | workers |
| 15 | `tool-runtime` | Tool sandbox controller (WASM/Docker/Firecracker lifecycle) | workers |
| 16 | `memory-service` | Episodic/semantic memory, embedding pipeline, pgvector search | kernel |

**Chat (v1):** Chat capability is built into `api-gateway`; there is no separate chat service for v1.

## Chat API (Phase 10)

| Method | Path | Description |
|---|---|---|
| `POST` | `/chat/sessions` | Create a chat session (agent_id, title) |
| `GET` | `/chat/sessions` | List chat sessions for user |
| `GET` | `/chat/sessions/{id}` | Get chat session details |
| `GET` | `/chat/ws` | WebSocket upgrade for streaming chat |

## Agent Profile & Documents API (Phase 9)

The following REST endpoints are added to the `api-gateway` for agent profile and document management:

| Method | Path | Description |
|---|---|---|
| `PATCH` | `/agents/{id}` | Update agent profile (system_prompt, config) |
| `GET` | `/agents/{id}/profile` | Get agent profile (served from Redis cache, 5min TTL) |
| `POST` | `/agents/{id}/documents` | Attach a document (rule/skill/context_doc/reference) to an agent |
| `GET` | `/agents/{id}/documents` | List agent documents, optional `?doc_type=` filter |
| `DELETE` | `/agents/{id}/documents/{doc_id}` | Remove a document |
| `POST` | `/goals` | (Updated) Accepts optional `documents` array for goal-scoped context |

**Context propagation flow:**
1. `goal-service` receives goal with optional inline documents → persists goal-scoped documents with `goal_id` set
2. `goal-service` assembles full agent context: `system_prompt` + rules (priority-sorted) + skills + context_docs
3. `goal-service` passes assembled `agent_context` to `planner-service`
4. `planner-service` embeds `agent_context` in each task payload
5. `execution-worker` includes `agent_context` when building LLM prompts for task execution

## Dashboard API (Phase 11)

The platform dashboard is served at `/superadmin/dashboard/`. Dashboard-specific APIs live under `/superadmin/api/dashboard/` and `/superadmin/api/slack/` (snapshot, approvals, goals, tasks, agents, chat sessions, Slack config). Multi-tenant (organizations, teams, org-level APIs) has been removed; the platform is single-tenant.

### Super-admin dashboard UI (implementation spec)

| Area | Requirement |
|------|-------------|
| **Assets** | Static files under `cmd/api-gateway/dashboard/` (`index.html`, `static/style.css`, `static/app.js`). Served by api-gateway at `/superadmin/dashboard/`. |
| **Shell** | Redesign layout (`body.dashboard-redesign`): glass-style topnav; logo with pastel lavender gradient on “Astra”; nav tabs **Overview** and **Slack**; **light/dark theme** toggle (persisted in `localStorage`, `data-theme` on `<html>`); Refresh; API Docs link. |
| **Visual language** | **Pastel** palette in both themes (lavender accent, mint/sky/butter/rose/peach for status and charts). Soft ambient gradients; avoid harsh saturated primaries. |
| **Light theme — readability** | Stat values and table content must meet readable contrast: dark numerals on stat cards; table body and mono cells legible; status pills with dark text on tinted backgrounds; agent action controls visible; cost column (`.cell-green`) dark green on white. Logs may retain dark terminal styling. |
| **Charts** | Chart.js; colors align with pastel tokens; legend and axis colors **follow active theme** (toggle must refresh chart styling). |
| **Features** | Stats grid; charts (tasks, goals, service health, agents); agents table with pagination and row actions; goals, tasks, workers, approvals, cost, logs; Slack app configuration tab; modals (agent create/edit, approvals); optional chat widget for chat-capable agents. |
| **Stack** | Vanilla HTML/CSS/JS for this surface (not React/MUI). Fonts: **Inter**, **Roboto Mono**. |

---

# 10. Protobuf & gRPC Contracts

## kernel.proto

```protobuf
syntax = "proto3";
package astra.kernel;
option go_package = "astra/proto/kernel";

service KernelService {
  rpc SpawnActor(SpawnActorRequest) returns (SpawnActorResponse);
  rpc SendMessage(SendMessageRequest) returns (SendMessageResponse);
  rpc QueryState(QueryStateRequest) returns (QueryStateResponse);
  rpc SubscribeStream(SubscribeStreamRequest) returns (stream Event);
  rpc PublishEvent(PublishEventRequest) returns (PublishEventResponse);
}

message SpawnActorRequest {
  string actor_type = 1;
  bytes config = 2;
}
message SpawnActorResponse {
  string actor_id = 1;
}

message SendMessageRequest {
  string target_actor_id = 1;
  string message_type = 2;
  string source = 3;
  bytes payload = 4;
}
message SendMessageResponse {}

message QueryStateRequest {
  string entity_type = 1;  // "agent", "task", "worker"
  map<string, string> filters = 2;
}
message QueryStateResponse {
  repeated bytes results = 1;
}

message SubscribeStreamRequest {
  string stream_name = 1;
  string consumer_group = 2;
  string consumer_id = 3;
}
message Event {
  string id = 1;
  string type = 2;
  string actor_id = 3;
  bytes payload = 4;
  int64 timestamp = 5;
}

message PublishEventRequest {
  string stream_name = 1;
  string event_type = 2;
  string actor_id = 3;
  bytes payload = 4;
}
message PublishEventResponse {
  string event_id = 1;
}
```

## task.proto

```protobuf
syntax = "proto3";
package astra.tasks;
option go_package = "astra/proto/tasks";

service TaskService {
  rpc CreateTask(CreateTaskRequest) returns (CreateTaskResponse);
  rpc ScheduleTask(ScheduleTaskRequest) returns (ScheduleTaskResponse);
  rpc CompleteTask(CompleteTaskRequest) returns (CompleteTaskResponse);
  rpc FailTask(FailTaskRequest) returns (FailTaskResponse);
  rpc GetTask(GetTaskRequest) returns (GetTaskResponse);
  rpc GetGraph(GetGraphRequest) returns (GetGraphResponse);
}

message CreateTaskRequest {
  string graph_id = 1;
  string agent_id = 2;
  string type = 3;
  bytes payload = 4;
  int32 priority = 5;
  repeated string depends_on = 6;
}
message CreateTaskResponse {
  string task_id = 1;
}

message ScheduleTaskRequest {
  string task_id = 1;
}
message ScheduleTaskResponse {}

message CompleteTaskRequest {
  string task_id = 1;
  bytes result = 2;
}
message CompleteTaskResponse {}

message FailTaskRequest {
  string task_id = 1;
  string error = 2;
}
message FailTaskResponse {}

message GetTaskRequest {
  string task_id = 1;
}
message GetTaskResponse {
  string id = 1;
  string graph_id = 2;
  string agent_id = 3;
  string type = 4;
  string status = 5;
  bytes payload = 6;
  bytes result = 7;
  int32 priority = 8;
  int32 retries = 9;
  int64 created_at = 10;
  int64 updated_at = 11;
}

message GetGraphRequest {
  string graph_id = 1;
}
message GetGraphResponse {
  repeated GetTaskResponse tasks = 1;
  repeated TaskDependency dependencies = 2;
}
message TaskDependency {
  string task_id = 1;
  string depends_on = 2;
}
```

---

# 11. Database Schema, Indexes & Migrations

## Extensions

```sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS vector;
```

## Migration 0001: Initial Schema

```sql
CREATE TABLE agents (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  name TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  config JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE goals (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
  goal_text TEXT NOT NULL,
  priority INT NOT NULL DEFAULT 100,
  status TEXT NOT NULL DEFAULT 'pending',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE tasks (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  graph_id UUID NOT NULL,
  goal_id UUID REFERENCES goals(id) ON DELETE SET NULL,
  agent_id UUID NOT NULL,
  type TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'created',
  payload JSONB DEFAULT '{}',
  result JSONB,
  priority INT NOT NULL DEFAULT 100,
  retries INT NOT NULL DEFAULT 0,
  max_retries INT NOT NULL DEFAULT 5,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

## Migration 0002: Task Dependencies

```sql
CREATE TABLE task_dependencies (
  task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  depends_on UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
  PRIMARY KEY (task_id, depends_on)
);
```

## Migration 0003: Memories (pgvector)

```sql
CREATE TABLE memories (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
  memory_type TEXT NOT NULL,
  content TEXT NOT NULL,
  embedding VECTOR(1536),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

## Migration 0004: Artifacts

```sql
CREATE TABLE artifacts (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  agent_id UUID,
  task_id UUID REFERENCES tasks(id) ON DELETE SET NULL,
  uri TEXT NOT NULL,
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

## Migration 0005: Workers

```sql
CREATE TABLE workers (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  hostname TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  capabilities JSONB DEFAULT '[]',
  last_heartbeat TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

## Migration 0006: Indexes

```sql
CREATE INDEX idx_goals_agent ON goals(agent_id);
CREATE INDEX idx_tasks_agent ON tasks(agent_id);
CREATE INDEX idx_tasks_status ON tasks(status);
CREATE INDEX idx_tasks_graph ON tasks(graph_id);
CREATE INDEX idx_task_dep_task ON task_dependencies(task_id);
CREATE INDEX idx_task_dep_dependson ON task_dependencies(depends_on);
CREATE INDEX idx_memories_agent ON memories(agent_id);
CREATE INDEX idx_memory_embedding ON memories USING ivfflat (embedding vector_cosine_ops);
CREATE INDEX idx_events_actor ON events(actor_id);
CREATE INDEX idx_events_type ON events(event_type);
CREATE INDEX idx_workers_status ON workers(status);
```

## Migration 0007: Events

```sql
CREATE TABLE events (
  id BIGSERIAL PRIMARY KEY,
  event_type TEXT NOT NULL,
  actor_id UUID,
  payload JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

## Migration 0008: Constraints & Triggers

```sql
ALTER TABLE tasks ADD CONSTRAINT tasks_valid_status
  CHECK (status IN ('created', 'pending', 'queued', 'scheduled', 'running', 'completed', 'failed'));

ALTER TABLE agents ADD CONSTRAINT agents_valid_status
  CHECK (status IN ('active', 'stopped', 'error'));

ALTER TABLE workers ADD CONSTRAINT workers_valid_status
  CHECK (status IN ('active', 'draining', 'offline'));

CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER agents_updated_at BEFORE UPDATE ON agents
  FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER tasks_updated_at BEFORE UPDATE ON tasks
  FOR EACH ROW EXECUTE FUNCTION update_updated_at();
```

## Migration 0009: Phase history, LLM usage, and audit

Supports phase/build history (file + DB + vector), per-request token/LLM usage persistence, and audit queries. See **docs/phase-history-usage-audit-design.md** for full design.

```sql
-- phase_runs: one row per phase (e.g. goal execution)
CREATE TABLE phase_runs (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  goal_id UUID REFERENCES goals(id) ON DELETE SET NULL,
  agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
  name TEXT,
  status TEXT NOT NULL DEFAULT 'running',
  started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ended_at TIMESTAMPTZ,
  summary TEXT,
  timeline JSONB DEFAULT '[]',
  log_file_path TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Constraint: status IN ('running', 'completed', 'failed', 'cancelled')

-- phase_summaries: pgvector for semantic search over "what was done"
CREATE TABLE phase_summaries (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  phase_id UUID NOT NULL REFERENCES phase_runs(id) ON DELETE CASCADE,
  content TEXT NOT NULL,
  embedding VECTOR(1536),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Index: ivfflat on embedding (same pattern as memories)

-- llm_usage: one row per LLM request for analytics and audit
CREATE TABLE llm_usage (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  request_id TEXT,
  agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
  task_id UUID REFERENCES tasks(id) ON DELETE SET NULL,
  model TEXT NOT NULL,
  tokens_in INT NOT NULL DEFAULT 0,
  tokens_out INT NOT NULL DEFAULT 0,
  latency_ms INT,
  cost_dollars NUMERIC(12,6),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Indexes: (agent_id, created_at), (task_id, created_at), (created_at)

-- Audit: time-range queries on events
CREATE INDEX idx_events_created_at ON events(created_at);
```

Idempotent migration: `migrations/0009_phase_history_and_usage.sql`. Phase history is also written to a human-readable file per phase (configurable path); development-phase history is maintained in **docs/phase-history/** (e.g. phase-0.md).

## Migration 0013: Agent Profile & Documents

```sql
-- Add system_prompt to agents for persona definition
ALTER TABLE agents ADD COLUMN IF NOT EXISTS system_prompt TEXT DEFAULT '';

-- Agent documents: rules, skills, context docs, and reference material attached to agents
CREATE TABLE IF NOT EXISTS agent_documents (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
  goal_id UUID REFERENCES goals(id) ON DELETE SET NULL,
  doc_type TEXT NOT NULL,
  name TEXT NOT NULL,
  content TEXT,
  uri TEXT,
  metadata JSONB DEFAULT '{}',
  priority INT NOT NULL DEFAULT 100,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE agent_documents ADD CONSTRAINT agent_documents_valid_doc_type
  CHECK (doc_type IN ('rule', 'skill', 'context_doc', 'reference'));

CREATE INDEX IF NOT EXISTS idx_agent_documents_agent ON agent_documents(agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_documents_goal ON agent_documents(goal_id);
CREATE INDEX IF NOT EXISTS idx_agent_documents_type ON agent_documents(agent_id, doc_type);

CREATE TRIGGER agent_documents_updated_at BEFORE UPDATE ON agent_documents
  FOR EACH ROW EXECUTE FUNCTION update_updated_at();
```

Idempotent migration: `migrations/0013_agent_profile_and_documents.sql`. Documents with `goal_id` set are scoped to that goal only. Documents without `goal_id` are global to the agent. Large document content may be stored in MinIO/S3 with a URI reference in the `uri` column.

## Migration 0014: Approval requests — plan type

Extends `approval_requests` to support two types: *plan* (implementation plan approval) and *risky_task* (dangerous tool execution). Adds `request_type`, `goal_id`, `graph_id`, `plan_payload` (JSONB). Idempotent migration: `migrations/0014_approval_requests_plan_type.sql`. See **docs/approval-system-extension-spec.md**.

## Migration 0015: Agents unique name

De-duplicates agents by `name` (keeps oldest per name), reassigns foreign keys to the kept agent, then adds `UNIQUE(agents.name)`. Idempotent migration: `migrations/0015_agents_unique_name.sql`. Standalone script `scripts/dedup-agents-by-name.sql` performs de-dup only (no constraint). Agent names are canonical and unique going forward.

## Migration 0016: Chat sessions and messages

Adds `chat_sessions` (user_id, agent_id, title, status, expires_at), `chat_messages` (session_id, role, content, tool_calls, tool_results, token counts), and `agents.chat_capable` column. Sessions and messages support real-time WebSocket chat with streaming and optional tool invocation. Idempotent migration: **migrations/0016_chat.sql**.

## Migration 0018: Multi-Tenancy

Converts Astra from single-tenant to multi-tenant with organizations, teams, users, roles, and tiered agent visibility. Idempotent migration: `migrations/0018_multi_tenant.sql`.

### New tables

```sql
-- Platform user accounts
CREATE TABLE IF NOT EXISTS users (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  email TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  is_super_admin BOOLEAN NOT NULL DEFAULT false,
  last_login_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE users ADD CONSTRAINT users_valid_status
  CHECK (status IN ('active', 'suspended', 'deactivated'));

-- Organizations (tenants)
CREATE TABLE IF NOT EXISTS organizations (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  name TEXT NOT NULL UNIQUE,
  slug TEXT NOT NULL UNIQUE,
  status TEXT NOT NULL DEFAULT 'active',
  config JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE organizations ADD CONSTRAINT orgs_valid_status
  CHECK (status IN ('active', 'suspended', 'archived'));

-- User-org membership with role
CREATE TABLE IF NOT EXISTS org_memberships (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  role TEXT NOT NULL DEFAULT 'member',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(user_id, org_id)
);
ALTER TABLE org_memberships ADD CONSTRAINT org_memberships_valid_role
  CHECK (role IN ('admin', 'member'));

-- Teams within orgs
CREATE TABLE IF NOT EXISTS teams (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  slug TEXT NOT NULL,
  description TEXT DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(org_id, slug)
);

-- User-team membership
CREATE TABLE IF NOT EXISTS team_memberships (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role TEXT NOT NULL DEFAULT 'member',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(team_id, user_id)
);
ALTER TABLE team_memberships ADD CONSTRAINT team_memberships_valid_role
  CHECK (role IN ('admin', 'member'));

-- Agent collaborator grants (user or team)
CREATE TABLE IF NOT EXISTS agent_collaborators (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
  collaborator_type TEXT NOT NULL,
  collaborator_id UUID NOT NULL,
  permission TEXT NOT NULL DEFAULT 'use',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(agent_id, collaborator_type, collaborator_id)
);
ALTER TABLE agent_collaborators ADD CONSTRAINT ac_valid_type
  CHECK (collaborator_type IN ('user', 'team'));
ALTER TABLE agent_collaborators ADD CONSTRAINT ac_valid_perm
  CHECK (permission IN ('use', 'edit', 'admin'));

-- Agent admins (receive approve/reject requests)
CREATE TABLE IF NOT EXISTS agent_admins (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(agent_id, user_id)
);
```

### ALTER existing tables

```sql
-- agents: tenant columns
ALTER TABLE agents ADD COLUMN IF NOT EXISTS org_id UUID REFERENCES organizations(id) ON DELETE CASCADE;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS owner_id UUID REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS team_id UUID REFERENCES teams(id) ON DELETE SET NULL;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS visibility TEXT NOT NULL DEFAULT 'private';
ALTER TABLE agents ADD CONSTRAINT agents_valid_visibility
  CHECK (visibility IN ('global', 'public', 'team', 'private'));
UPDATE agents SET visibility = 'global' WHERE org_id IS NULL;

-- agents: per-agent external ingest and Slack (migration 0022)
-- ingest_source_type: redis_pubsub | gcp_pubsub | websocket (NULL = none)
-- ingest_source_config: JSONB for channel/project/subscription/url
-- slack_notifications_enabled: allow agent to post to Slack when instructions say so
ALTER TABLE agents ADD COLUMN IF NOT EXISTS ingest_source_type TEXT;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS ingest_source_config JSONB;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS slack_notifications_enabled BOOLEAN NOT NULL DEFAULT false;
-- slack_workspaces (migration 0021): notification_channel_id for default proactive post channel
-- See migrations/0022_agent_ingest_and_slack.sql, migrations/0021_slack_notification_channel.sql

-- goals
ALTER TABLE goals ADD COLUMN IF NOT EXISTS org_id UUID REFERENCES organizations(id) ON DELETE CASCADE;
ALTER TABLE goals ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id) ON DELETE SET NULL;

-- tasks
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS org_id UUID REFERENCES organizations(id) ON DELETE CASCADE;

-- workers
ALTER TABLE workers ADD COLUMN IF NOT EXISTS org_id UUID REFERENCES organizations(id) ON DELETE CASCADE;

-- events, memories, llm_usage, approval_requests, chat_sessions
ALTER TABLE events ADD COLUMN IF NOT EXISTS org_id UUID;
ALTER TABLE memories ADD COLUMN IF NOT EXISTS org_id UUID;
ALTER TABLE llm_usage ADD COLUMN IF NOT EXISTS org_id UUID;
ALTER TABLE llm_usage ADD COLUMN IF NOT EXISTS user_id UUID;
ALTER TABLE approval_requests ADD COLUMN IF NOT EXISTS org_id UUID;
ALTER TABLE approval_requests ADD COLUMN IF NOT EXISTS requested_by UUID REFERENCES users(id);
ALTER TABLE approval_requests ADD COLUMN IF NOT EXISTS assigned_to UUID REFERENCES users(id);
ALTER TABLE chat_sessions ADD COLUMN IF NOT EXISTS org_id UUID;
```

### Multi-tenant indexes

```sql
CREATE INDEX IF NOT EXISTS idx_org_memberships_user ON org_memberships(user_id);
CREATE INDEX IF NOT EXISTS idx_org_memberships_org ON org_memberships(org_id);
CREATE INDEX IF NOT EXISTS idx_teams_org ON teams(org_id);
CREATE INDEX IF NOT EXISTS idx_team_memberships_team ON team_memberships(team_id);
CREATE INDEX IF NOT EXISTS idx_team_memberships_user ON team_memberships(user_id);
CREATE INDEX IF NOT EXISTS idx_agents_org ON agents(org_id);
CREATE INDEX IF NOT EXISTS idx_agents_owner ON agents(owner_id);
CREATE INDEX IF NOT EXISTS idx_agents_visibility ON agents(visibility);
CREATE INDEX IF NOT EXISTS idx_goals_org ON goals(org_id);
CREATE INDEX IF NOT EXISTS idx_tasks_org ON tasks(org_id);
CREATE INDEX IF NOT EXISTS idx_workers_org ON workers(org_id);
CREATE INDEX IF NOT EXISTS idx_events_org ON events(org_id);
CREATE INDEX IF NOT EXISTS idx_agent_collaborators_agent ON agent_collaborators(agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_admins_agent ON agent_admins(agent_id);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);
```

### Triggers

```sql
CREATE TRIGGER users_updated_at BEFORE UPDATE ON users
  FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER organizations_updated_at BEFORE UPDATE ON organizations
  FOR EACH ROW EXECUTE FUNCTION update_updated_at();
```

---

# 12. Message & Event Protocols

## Actor Message Format

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "type": "TaskScheduled",
  "source": "scheduler-01",
  "target": "worker-pool",
  "payload": {"task_id": "...", "graph_id": "...", "priority": 100},
  "meta": {"trace_id": "abc123"},
  "timestamp": "2026-03-09T10:30:00Z"
}
```

## Core Message Types

| Category | Types |
|---|---|
| Goal lifecycle | `GoalCreated`, `PlanRequested`, `PlanGenerated` |
| Task lifecycle | `TaskCreated`, `TaskScheduled`, `TaskStarted`, `TaskCompleted`, `TaskFailed` |
| Phase lifecycle | `PhaseStarted`, `PhaseCompleted`, `PhaseFailed`, `PhaseSummary` |
| LLM usage & audit | `LLMUsage` (model, tokens in/out, latency, cost; appended to `events` for audit) |
| Memory | `MemoryWrite`, `MemoryRead` |
| Tool execution | `ToolExecutionRequested`, `ToolExecutionCompleted` |
| Evaluation | `EvaluationRequested`, `EvaluationCompleted` |
| Agent lifecycle | `AgentSpawned`, `AgentStopped`, `AgentError` |
| Worker | `WorkerHeartbeat`, `WorkerRegistered`, `WorkerDraining` |

## Redis Streams

### 1. `astra:events` — Global event stream
| Field | Type | Description |
|---|---|---|
| `event_id` | UUID | Unique event ID |
| `type` | string | Event type |
| `actor_id` | UUID | Originating actor |
| `payload` | JSON | Event data |
| `timestamp` | RFC3339 | Event time |

### 2. `astra:tasks:shard:<n>` — Shard-specific task queues
| Field | Type | Description |
|---|---|---|
| `task_id` | UUID | Task ID |
| `graph_id` | UUID | Parent graph |
| `agent_id` | UUID | Owning agent |
| `task_type` | string | Task classifier |
| `payload` | JSON | Task payload |
| `priority` | int | Priority (lower = higher) |
| `created_at` | RFC3339 | Creation time |

### 3. `astra:agent:events` — Agent lifecycle events
Fields: `agent_id`, `event_type`, `payload`, `timestamp`

### 4. `astra:worker:events` — Worker events and heartbeats
Fields: `worker_id`, `event_type`, `task_id`, `metadata`, `timestamp`

### 5. `astra:evaluation` — Evaluation results
Fields: `task_id`, `evaluator_id`, `result`, `metadata`, `timestamp`

### 6. `astra:usage` — LLM usage (async persistence for audit)
Fields: `request_id`, `agent_id`, `task_id`, `org_id`, `user_id`, `model`, `tokens_in`, `tokens_out`, `latency_ms`, `cost_dollars`, `timestamp`. Consumer writes to `llm_usage` and appends to `events` with type `LLMUsage`. Keeps API under 10 ms (no synchronous DB write on LLM response path).

### Multi-tenancy stream fields
All streams carry `org_id` when available. `astra:tasks:shard:<n>` includes `org_id` from the task. `astra:usage` includes `org_id` and `user_id`. `astra:agent:events` includes `org_id`. This enables org-scoped consumers and audit filtering.

All streams use consumer groups. Messages acknowledged after processing. Dead letter on 3 failed processing attempts.

## Redis Streams — Go Implementation

```go
package messaging

import (
    "context"
    "fmt"
    "time"

    "github.com/redis/go-redis/v9"
)

type Bus struct {
    client *redis.Client
}

func New(addr string) *Bus {
    return &Bus{
        client: redis.NewClient(&redis.Options{
            Addr:         addr,
            DialTimeout:  2 * time.Second,
            ReadTimeout:  3 * time.Second,
            WriteTimeout: 3 * time.Second,
            PoolSize:     50,
        }),
    }
}

func (b *Bus) Publish(ctx context.Context, stream string, fields map[string]interface{}) error {
    return b.client.XAdd(ctx, &redis.XAddArgs{
        Stream: stream,
        MaxLen: 1000000,
        Approx: true,
        Values: fields,
    }).Err()
}

func (b *Bus) Consume(ctx context.Context, stream, group, consumer string, handler func(redis.XMessage) error) error {
    // Ensure consumer group exists
    b.client.XGroupCreateMkStream(ctx, stream, group, "0")

    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
        }
        msgs, err := b.client.XReadGroup(ctx, &redis.XReadGroupArgs{
            Group:    group,
            Consumer: consumer,
            Streams:  []string{stream, ">"},
            Count:    10,
            Block:    5 * time.Second,
        }).Result()
        if err != nil {
            time.Sleep(100 * time.Millisecond)
            continue
        }
        for _, msg := range msgs[0].Messages {
            if err := handler(msg); err != nil {
                continue // will be reclaimed via XAUTOCLAIM
            }
            b.client.XAck(ctx, stream, group, msg.ID)
        }
    }
}
```

---

# 13. Caching & Fast-path — Redis + Memcached

## Redis Keys

| Key Pattern | Type | TTL | Purpose |
|---|---|---|---|
| `actor:state:<actor_id>` | Hash | 5m | Working memory for active actors |
| `lock:task:<task_id>` | String (SET NX PX) | 30s | Distributed lock for task claiming (Redlock) |
| `worker:heartbeat:<worker_id>` | String | 30s | Worker liveness tracking |
| `agent:profile:<agent_id>` | Hash | 5m | Cached agent profile (system_prompt, config) |
| `agent:docs:<agent_id>` | String (JSON) | 5m | Cached agent documents list (rules, skills, context_docs) |
| `user:<user_id>` | Hash | 5m | Cached user profile (email, name, is_super_admin) |
| `user:orgs:<user_id>` | String (JSON) | 5m | Cached org memberships for user |
| `org:members:<org_id>` | String (JSON) | 5m | Cached org member list |
| `org:teams:<org_id>` | String (JSON) | 5m | Cached team list for org |

## Memcached Keys

| Key Pattern | TTL | Purpose |
|---|---|---|
| `llm:resp:{model}:{prompt_hash}` | 24h | Cached LLM responses |
| `embed:{content_hash}` | 7-30d | Cached embedding vectors |
| `tool:cache:{tool_name}:{input_hash}` | varies | Cached tool execution results |

## Cache Strategy

- **All API read endpoints** serve from Redis or Memcached — never Postgres
- **Write path**: persist to Postgres → emit event to Redis Stream → async cache update
- Cache misses fall through to Postgres but are populated for subsequent reads
- Use `MAXLEN ~` on streams to prevent unbounded growth

---

# 14. Tool Runtime & Sandboxing

## Sandbox Types

| Type | Use Case | Isolation Level |
|---|---|---|
| **WASM** | Lightweight safe plugins | Process-level |
| **Docker container** | Common tools needing filesystem/processes | Container-level |
| **Firecracker microVM** | Untrusted code, build tasks | VM-level |

## Tool Runtime Service Responsibilities

1. Launch sandbox with least-privileged role and isolated network (egress controls)
2. Enforce resource limits (CPU, memory, disk, time)
3. Provision secrets via ephemeral volumes (never env vars, never logs)
4. Capture stdout/stderr and return as artifacts
5. Policy engine check before execution (`access-control` service, OPA-based)

## Go Interface

```go
package tools

import "context"

type SandboxType string

const (
    SandboxWASM        SandboxType = "wasm"
    SandboxDocker      SandboxType = "docker"
    SandboxFirecracker SandboxType = "firecracker"
)

type ToolRequest struct {
    Name        string
    Input       []byte
    Sandbox     SandboxType
    Timeout     time.Duration
    MemoryLimit int64
    CPULimit    float64
}

type ToolResult struct {
    Output    []byte
    Artifacts []string // URIs
    ExitCode  int
    Duration  time.Duration
}

type Runtime interface {
    Execute(ctx context.Context, req ToolRequest) (ToolResult, error)
}
```

---

# 15. Astra SDK — Agent API

## Core Primitives

```go
type Goal struct {
    ID       uuid.UUID
    Text     string
    Priority int
}

type AgentContext interface {
    ID() uuid.UUID
    Memory() MemoryClient
    CreateTask(t Task) error
    PublishEvent(ev Event) error
    CallTool(name string, input json.RawMessage) (ToolResult, error)
}

type MemoryClient interface {
    Write(agentID uuid.UUID, memType string, content string, embedding []float32) error
    Search(agentID uuid.UUID, query string, topK int) ([]Memory, error)
    GetByID(id uuid.UUID) (*Memory, error)
}
```

## Agent Skeleton

```go
type SimpleAgent struct {
    id uuid.UUID
}

func (a *SimpleAgent) Plan(ctx AgentContext, goal Goal) ([]Task, error) {
    // Use LLM to decompose goal into tasks
    // Return task list with dependencies
}

func (a *SimpleAgent) Execute(ctx AgentContext, t Task) (Result, error) {
    // Execute a single task
    // May call tools, write memory, publish events
}

func (a *SimpleAgent) Reflect(ctx AgentContext, outcome Outcome) error {
    // Learn from outcome, update memory
}
```

---

# 16. Agent Taxonomy & Workflows

## Core Agent Types

| Agent | Responsibility |
|---|---|
| PRD-Parser Agent | Parse structured requirements into features |
| Planner Agent | Generate Task Graphs (DAGs) from goals |
| Backend Dev Agent | Create API endpoints, generate code, run unit tests |
| Frontend Dev Agent | Scaffold UI components and pages |
| Integration Agent | Wire services and test E2E |
| Testing Agent | Generate tests, run CI pipelines |
| Debugging Agent | Triage failing tests and propose fixes |
| DevOps Agent | Generate infra config, deploy pipelines |
| Learning Agent | Optimize prompts and strategy based on feedback |

## Example Workflow (build from PRD)

```
1. PRD-Parser extracts features → GoalCreated events
2. Planner builds DAG → PlanGenerated event
3. Scheduler dispatches tasks to worker pool
4. Backend/Frontend agents execute code tasks
5. Evaluation service validates results
6. Debugging agents fix issues, re-run failed tasks
7. DevOps agent deploys on successful pipeline
8. Learning agent optimizes prompts for next iteration
```

## Chat agents (design)

Agents that users connect to via **WebSocket** for real-time chat with **streaming** responses (e.g. token-by-token). Chat agents can invoke the same workers and tool runtime as goal-based agents. Design: **docs/chat-agents-design.md** (session model, protocol, auth, tool calls). Implementation plan: **docs/chat-agents-implementation-plan.md**.

**WebSocket protocol:** JSON frames include `chunk`, `message_start`, `message_end`, `tool_call`, `tool_result`, `done`, `error`, `pong`, and `session`. Session model: each chat is a session (user + agent) with a message history; sessions are created via REST and upgraded to WebSocket at `/chat/ws`.

## Slack integration (design)

Users can interact with Astra chat agents from **Slack**. One Slack workspace is linked to the platform via OAuth. **Slack app secrets** (Signing Secret, Client ID, Client Secret, OAuth Redirect URL) are configurable from the **super-admin UI** (stored encrypted in DB; env fallback). A dedicated **slack-adapter** service receives Slack Events API (and optional slash commands), verifies request signature, resolves org/agent/user from workspace and optional channel bindings, and enqueues work to a Redis stream for async processing. A worker consumes the stream, calls existing chat (and optionally goal) APIs with org-scoped identity, and posts replies back to Slack via the Slack API. Data: `slack_workspaces`, `slack_channel_bindings`, `slack_user_mappings`; bot tokens stored in Vault. Design: **docs/slack-integration-design.md**.

**Proactive posting:** Astra supports **proactively posting** to Slack (e.g. “Plan pending approval”) without a prior user message. The api-gateway exposes **`POST /internal/slack/post`** (internal only; auth via header **`X-Slack-Internal-Secret`**). Body: optional `channel_id`, `text` (required), optional `thread_ts`. If `channel_id` is omitted, the default workspace's notification channel is used when set. Token refresh on 401 is handled by `internal/slack`. Callers (e.g. goal-service, access-control) may call this endpoint to notify users in Slack; they need the gateway URL and internal secret in their environment.

---

# 17. Observability, Tracing, Metrics

## Stack

- **Tracing**: OpenTelemetry SDK → OTLP exporter → collector → Jaeger/Tempo
- **Metrics**: Prometheus (pull-based) with Go client
- **Logs**: Structured JSON (`slog`/`zerolog`) → Loki or Elasticsearch
- **Dashboards**: Grafana

## Metrics

| Metric | Type | Description |
|---|---|---|
| `astra_task_latency_seconds` | Histogram | Task execution latency |
| `astra_task_success_total` | Counter | Successful task completions |
| `astra_task_failure_total` | Counter | Failed task executions |
| `astra_events_processed_total` | Counter | Events processed by consumers |
| `astra_actor_count` | Gauge | Currently running actors |
| `astra_worker_heartbeat_total` | Counter | Worker heartbeats received |
| `astra_llm_token_usage_total` | Counter | LLM tokens consumed |
| `astra_llm_cost_dollars` | Counter | LLM cost in dollars |
| `astra_scheduler_ready_queue_depth` | Gauge | Tasks waiting to be scheduled |

## Per-request usage and audit

Every request that triggers an LLM call returns **token/LLM usage** in the response (model, tokens in/out, latency, cost if available) via the response envelope (e.g. gRPC metadata or response field). Persistence is **asynchronous**: usage is published to the `astra:usage` stream; a consumer writes to `llm_usage` and appends `events` with type `LLMUsage` so the hot path stays under 10 ms. This gives the user visible usage per request and a full audit trail. See **docs/phase-history-usage-audit-design.md**.

## Tracing

- Each `Task` execution creates a root span
- Tool calls are child spans with resource attributes (tool name, sandbox type, CPU time)
- `trace_id` ties logs ↔ traces ↔ events

## Dashboards

| Dashboard | Content |
|---|---|
| **Super-Admin Dashboard** (`/superadmin/dashboard/`) | Platform-wide **redacted** view: service health, workers (names/status), agents (names/status), goal/task counts, LLM cost totals. **Organizations** section (create/edit/delete, add org admins). **Users** section (paginated table of ALL users, search/filter, suspend/activate/reset-password/role-change/move-org actions, detail modal). Org filter dropdown. No execution details, code, or chat messages visible. |
| **Platform Dashboard** (`/superadmin/dashboard/`) | Single dashboard: overview, agents, goals, workers, approvals, cost, Slack config, chat. |
| Cluster Overview | Capacity, active agents, task throughput, error rate |
| Agent Health | Per-agent throughput, avg latency, failure rate |
| Cost | LLM token usage & cost per agent, per model |
| Task Graph Viewer | Interactive DAG visualization per goal |

## Alerts

- High task failure rate (>5% over 5min)
- High task queue depth (>10k pending tasks)
- Low worker availability (<50% of registered workers)
- LLM cost spike (>2x daily average)

---

# 18. Security, Policy, Governance

## Authentication & Identity

- `identity` service handles user accounts (CRUD, login, password reset), service accounts, and JWT issuance
- JWT tokens carry claims: `user_id`, `email`, `is_super_admin`, `scopes`
- Login: `POST /users/login` with email + password → JWT with org context
- Multi-org users select which org to authenticate against
- Password hashing: bcrypt (`golang.org/x/crypto/bcrypt`)
- mTLS for all service-to-service communication
- `last_login_at` tracked on users table for audit

## Authorization

- `access-control` service implements role-based authorization via `internal/rbac`:
  - **Super-admin**: platform management (orgs, all users); execution detail access DENIED
  - **Authenticated user**: access to platform resources
  - **Org-member**: access per agent visibility rules (global/public/team/private)
  - **Team-admin**: manage team membership, create team-scoped agents
  - **Agent-admin**: approve/reject for specific agent
- Agent visibility enforced by `CanAccessAgent()` in `internal/rbac/visibility.go`
- Access control enforced by RBAC middleware and approval workflows
- Super-admin data redaction: `redactForSuperAdmin()` strips execution details, code, system_prompt, goal_text, payload, result, chat messages
- Dangerous tool approval gates remain (plan approval and risky_task approval); approval routing goes to agent_admins first, then org_admins

## Secrets

- Secrets injected at runtime from Vault
- Never persisted in code, logs, events, or artifact metadata
- Tool sandboxes receive secrets via ephemeral volumes only
- Phase 7 implementation adds Vault config overlay and TLS secret path configuration in `pkg/config`

## Tool & Action Governance

- Dangerous actions (delete infra, change prod, data deletion) require approval gates (human-in-the-loop)
- `policy-engine` intercepts tool execution events and allows/denies based on policy
- **Two approval types:** (1) **Plan** — when `AUTO_APPROVE_PLANS` is false, goal-service creates an approval request for the implementation plan before creating the task graph; access-control exposes it and calls goal-service `POST /internal/apply-plan` on approve. (2) **Risky task** — dangerous tool execution (e.g. `terraform apply`, certain `shell_exec`) creates an `approval_requests` row with `request_type='risky_task'`; tool-runtime waits for approval before running. Full design: **docs/approval-system-extension-spec.md**.

## Audit & Compliance

- **Immutable event log** — The `events` table is the canonical audit log. It includes task/agent lifecycle events and, for phase/usage audit: `PhaseStarted`, `PhaseCompleted`, `PhaseFailed`, `PhaseSummary`, and `LLMUsage` (per-request model, tokens in/out, latency, cost). Index on `created_at` supports time-range audit queries.
- **Phase/build history** — Each phase run is recorded in `phase_runs` (and optionally in `phase_summaries` for semantic search) and as a human-readable file per phase; phase lifecycle events are appended to `events`. Development-phase history (what was built in each implementation phase) is maintained in **docs/phase-history/** (e.g. phase-0.md).
- **Per-request LLM usage** — Visible to the user in the response; persisted asynchronously to `llm_usage` and to `events` as `LLMUsage` for analytics and audit.
- **Security compliance status** — S1 (mTLS transport) and S5 (Vault-backed secret management) are implemented in Phase 7 and validated in `scripts/validate.sh`.
- Artifact immutability (once written, cannot be modified).
- All state transitions logged with actor_id and timestamp.

Full design: **docs/phase-history-usage-audit-design.md**.

---

# 19. Multi-Tenancy Architecture

Astra is a **single-platform** autonomous agent OS: users and agents share one platform; there are no organizations or teams.

## Entity Model

- **Users** — Platform accounts with email/password authentication.
- **Organizations** — Tenants. Each org has its own agents, goals, tasks, workers, cost tracking, and approvals. One org's data is never visible to another org.
- **Teams** — Groups within an org. Teams can own agents and be granted collaborator access to agents.
- **Org Memberships** — Links users to orgs with a role (`admin` or `member`).
- **Team Memberships** — Links users to teams with a role (`admin` or `member`).

## Role Model

| Role | Scope | Permissions |
|---|---|---|
| `super_admin` | Platform | Manage ALL orgs and ALL users across ALL orgs (CRUD, suspend, role changes, move between orgs). See platform-wide metrics and specs (agent names, worker names, counts, cost totals). **Cannot** see execution details, code, goal text, task payloads, results, system prompts, or chat messages. Create/manage global agents. |
| `org_admin` | Organization | Full access to 100% of everything within the org: agents, goals, tasks, workers, execution details, code, chat. Manage teams and members within the org. |
| `org_member` | Organization | Use agents per visibility rules. Create private and public agents. Submit goals. |
| `team_admin` | Team | Manage team membership. Create team-scoped agents. |
| `team_member` | Team | Use team-scoped agents. |
| `agent_admin` | Agent | Receive and decide approve/reject requests for that specific agent's plans and risky tasks. |

## Agent Visibility Hierarchy

| Visibility | Scope | Who can see/use |
|---|---|---|
| `global` | Platform | Every user in every org. Only super-admins can create/modify. Current pre-existing agents are global. |
| `public` | Organization | Every member of the org. |
| `team` | Team | Team members and org admins only. |
| `private` | User | Only the owner and explicitly added collaborators (users or teams). |

### Agent Access (single-platform)

```go
func CanAccessAgent(claims Claims, agent Agent) bool {
    if claims.UserID == "" { return false }
    if claims.IsSuperAdmin { return true }
    return true // single-platform: all authenticated users can access agents
}
```

## Agent Collaborators & Agent Admins

- **Collaborators** grant access to private/team agents. A collaborator can be a user or a team. Permission levels: `use`, `edit`, `admin`.
- **Agent admins** are users designated to receive and decide approve/reject requests for a specific agent's plans and risky tasks. Falls back to org admins if no agent admins are set.

## Privacy & Data Isolation

1. **Authentication**: All dashboard and agent APIs require a valid JWT. Super-admin flag enables dashboard and certain admin actions.
2. **Super-admin redaction**: A `redactForSuperAdmin()` function strips `system_prompt`, `config`, `payload`, `result`, `goal_text`, code, file contents, shell output, and chat messages before returning data to super-admin endpoints.
3. **Single platform**: All services operate on a single platform; no tenant isolation. Agent-service `QueryState` returns all agents. Goal-service, task-service, worker-manager, cost-tracker, and memory-service query across the full dataset.
4. **Workspace isolation**: All goals use `WORKSPACE_ROOT/_global/{goal_id}/`.

## JWT Claims

```go
type Claims struct {
    jwt.RegisteredClaims
    UserID       string   `json:"user_id"`
    Email        string   `json:"email"`
    IsSuperAdmin bool     `json:"is_super_admin"`
    Scopes       []string `json:"scopes"`
}
```

Login flow: `POST /users/login` (or `/login` via gateway) with email + password. Identity looks up user and issues JWT with user_id, email, is_super_admin, scopes.

## URL Structure

| URL Pattern | Purpose | Auth |
|---|---|---|
| `/` | Landing/login page | None |
| `/login` | Login form | None |
| `/superadmin/dashboard` | Super-admin dashboard | super_admin JWT |
| `/superadmin/api/*` | Dashboard API (snapshot, approvals, goals, agents, chat, Slack config) | JWT |
| `/health` | Health check | None |

## Super-Admin Dashboard

The current dashboard at `/dashboard/` is renamed to `/superadmin/dashboard/`. It shows:

- **Organizations**: list, create/edit/delete, add org admins
- **Users**: paginated table of ALL platform users with search/filter (org, status, role). Actions: suspend, activate, reset password, change role, move between orgs. Click row for detail modal (org memberships, team memberships, owned agents, last login).
- **Platform metrics**: service health, agents (names/status only), workers (names/status only), goal/task counts, LLM cost totals — all redacted (no execution details).
- Org filter dropdown to narrow stats by org.

## (Removed) Org Dashboard

Org-admin-only. Shows 100% of org data:

- **Members**: list, invite, change roles, remove
- **Teams**: create, manage membership
- **Agents**: full list (all visibilities), create, edit visibility, manage collaborators/admins
- **Goals**: full detail (goal_text, task payloads, execution results, code)
- **Workers**: full detail
- **Approvals**: pending for org agents
- **Cost**: org-scoped LLM usage

## (Removed) Org Home

All org members see: agents they can access (filtered by visibility), recent goals, quick-submit goal form, chat (if enabled).

---

# 20. Deployment Architecture & Scaling

## Kubernetes Namespaces

| Namespace | Services |
|---|---|
| `control-plane` | api-gateway, identity, access-control |
| `kernel` | scheduler-service, task-service, agent-service, goal-service |
| `workers` | execution-worker, browser-worker, tool-runtime, worker-manager, llm-router, prompt-manager, evaluation-service |
| `infrastructure` | postgres (primary + replicas), redis (cluster), memcached (clustered); **local:** minio optional; **GCP:** Cloud Storage bucket (no MinIO) |
| `observability` | prometheus, grafana, opentelemetry-collector, loki |

## Scaling Model

| Component | Strategy |
|---|---|
| Stateless services | HPA based on CPU / request rate |
| Workers | HPA based on Redis queue depth + scheduler hints |
| Redis | Cluster mode, shard count based on throughput |
| Postgres | Primary for writes, read replicas for heavy reads |
| Memcached | Memory-optimized nodes, horizontal scaling |

## Helm Chart Structure

```yaml
# deployments/helm/astra/values.yaml
replicaCount:
  apiGateway: 2
  scheduler: 2
  worker: 4
  identity: 1
  accessControl: 1

image:
  repository: astra
  tag: latest

postgres:
  host: postgres.infrastructure
  port: 5432
  database: astra

redis:
  addr: redis.infrastructure:6379

memcached:
  addr: memcached.infrastructure:11211
```

## GCP deployment (managed services)

| Item | Detail |
|------|--------|
| **Entry script** | `scripts/gcp-deploy.sh` (repo root). **Not** `scripts/deploy.sh` (local only). |
| **Configuration** | Optional `.env.gcp`; template `scripts/.env.gcp.example` (`GCP_PROJECT`, `GCP_REGION`, `GCP_CLUSTER`, `POSTGRES_PASSWORD`, optional `GCS_WORKSPACE_BUCKET`). |
| **Flags** | `--setup` — first-time provision; `--dev` / `--prod` — tier (`values-gke-dev.yaml` vs `values-gke-prod.yaml`); `--build-only` — images only; `--deploy-only` — migrate + Helm without rebuild. |
| **Provisioned resources** | GKE Autopilot; Cloud SQL (PostgreSQL 15); Memorystore for Redis; Memorystore for Memcached; Artifact Registry; **Google Cloud Storage** bucket `gs://${GCP_PROJECT}-astra-workspace` (override via `GCS_WORKSPACE_BUCKET`). |
| **Object storage policy** | On the GCP deploy path, **workspace/artifact storage is GCS**. MinIO is for local/docker-compose only; do not rely on MinIO in production GCP. |
| **Application deploy** | Per-service `helm upgrade --install astra-<service>` using chart `deployments/helm/astra` with `--set service.name=<service>` and images from Artifact Registry. |
| **Documentation** | `README.md` (GCP section), `deployments/helm/astra/README.md`, `docs/deployment-design.md`. |

## Platform & Hardware Acceleration

Astra supports **platform-aware execution** so it runs efficiently and natively on the host OS while remaining portable.

### macOS (Apple Silicon / Intel)

When running on **darwin** (macOS):

- **Production deployment** — macOS is a supported **production** deployment target. Astra can be run in production on Mac hardware (e.g. Mac Mini, Mac Studio, or on-prem Mac servers) for development, edge, or dedicated Mac-based deployments. Same services and APIs as Linux; deploy via native binaries, process managers (e.g. launchd), or container runtimes that support macOS.
- **Metal** — Use for GPU-accelerated workloads where supported: local inference, embedding computation, and other compute-heavy paths. Prefer Metal-backed runtimes (e.g. Metal-accelerated inference or embedding backends) when available so Astra benefits from Apple GPU without requiring CUDA.
- **Neural Engine (ANE)** — When exposed via stable APIs (e.g. Core ML, or framework support for ANE), use for inference and embedding workloads to reduce power and latency on supported Macs. Detection and fallback to Metal or CPU must be explicit.
- **Native binaries** — Build and ship `darwin/arm64` and `darwin/amd64` so Macs run native binaries. No emulation on Apple Silicon.

Detection is via runtime (`runtime.GOOS == "darwin"`) or build tags; backend selection (Metal vs CPU, ANE when available) is explicit and fallback is always to a working path (e.g. CPU).

### Linux (production and on-prem)

When running on **linux**:

- **Production deployment** — Linux is the primary production deployment target for cloud and data-center environments (Kubernetes, Docker, bare metal). Container images and Helm charts target Linux.
- **CPU** — Default path. All services run correctly on CPU-only Linux.
- **CUDA** — When NVIDIA GPUs are available, support CUDA-backed inference and embeddings (e.g. via existing runtimes or connectors). Detection via env, capability probes, or config; no dependency on macOS-only APIs.
- **Portability** — No use of Metal or other macOS-only APIs on Linux. **Both Linux and macOS are supported for production;** choose the platform that fits your environment (e.g. K8s on Linux, native or containerized on macOS).

### Design principles

1. **Single codebase** — One codebase for both platforms. Use abstraction (e.g. backend interface, build tags, or platform-specific implementations behind a common API) so that Go code chooses the right backend at runtime or build time.
2. **Graceful fallback** — If Metal, ANE, or CUDA is unavailable or disabled, fall back to CPU. No hard requirement for specific hardware.
3. **Explicit opt-in** — Hardware acceleration can be enabled/disabled via config or env (e.g. `ASTRA_USE_METAL=true`, `ASTRA_USE_CUDA=false`) so operators can tune for cost or compatibility.
4. **Observability** — Log or expose metrics for which backend is active (e.g. `inference_backend=metal|cuda|cpu`) so behavior is debuggable in production and on Mac.

### Scope

- **In-scope:** Backend selection for inference and embedding pipelines used by Astra (including local model runtimes and connectors), optional Metal/ANE on macOS, CUDA on Linux, native darwin/linux binaries, and **production deployment on both macOS and Linux** (native or containerized per platform).
- **Out-of-scope:** Implementing custom Metal or CUDA kernels inside Astra core; Astra integrates with existing runtimes and connectors that support these backends.

---

# 21. Failure Modes, Recovery & Runbooks

| Failure | Detection | Recovery |
|---|---|---|
| Worker crash | Heartbeat lost (>30s) | Requeue in-flight tasks, restart worker |
| Postgres outage | Connection error | Read-only mode, promote replica |
| Redis failure | Connection error | Failover to replica, replay from `events` table |
| Task graph corruption | Inconsistent state | Reconstruct via event-sourcing replay |
| LLM cost spike | Budget alert | Disable premium model routing, enforce lower tiers |
| Scheduler shard imbalance | Monitoring | Rebalance shards via consistent hashing |

## Runbook: Worker Lost

1. Identify last heartbeat: `redis-cli GET worker:heartbeat:<worker_id>`
2. Check worker events: `redis-cli XRANGE astra:worker:events - + COUNT 20`
3. Move in-flight tasks: `UPDATE tasks SET status='queued' WHERE worker_id=$1 AND status='running'`
4. Restart worker pod: `kubectl rollout restart deployment/execution-worker -n workers`

## Runbook: High Error Rate

1. Sample failed tasks: `SELECT * FROM tasks WHERE status='failed' ORDER BY updated_at DESC LIMIT 20`
2. Examine traces: correlate `trace_id` from events to Jaeger/Tempo
3. If widespread: pause goal intake via API gateway feature flag
4. Roll back last deployment if caused by code change

---

# 22. CI/CD, Testing & Release Plan

## CI Pipeline

1. `go vet ./...`
2. `golangci-lint run ./...`
3. `buf lint` (proto validation)
4. `go test ./... -race -count=1` (unit tests)
5. Integration tests with `testcontainers-go` (Postgres, Redis)
6. Contract tests for protobuf APIs

## Staging Pipeline

1. Build Docker images for all changed services
2. Deploy to staging cluster
3. Run full integration + simulated agent workloads (test DAGs)
4. Canary deploy to production (5% traffic), monitor 30 minutes
5. Full rollout

## Release Cadence

- Weekly minor deploys (feature-gated)
- Quarterly major releases (schema migrations planned with versioned migration tool)

---

# 23. Cost Management & LLM Routing

## LLM Router Logic

```go
package llm

type ModelTier string

const (
    TierLocal   ModelTier = "local"
    TierPremium ModelTier = "premium"
    TierCode    ModelTier = "code"
)

type Router struct{}

func (r *Router) Route(taskType string, priority int) ModelTier {
    switch {
    case taskType == "classification":
        return TierLocal
    case taskType == "code_generation":
        return TierCode
    case priority < 50:
        return TierPremium
    default:
        return TierLocal
    }
}
```

## Cost Controls

- Token quota per agent / per organization
- Burst budget with hard caps and admin approval
- Response caching via Memcached for repeated prompts
- Batch inference where possible (GPU worker)

## Cost Monitoring

- `cost_tracking` monitors token usage, model calls per agent, cost per task
- Alerts when thresholds exceeded
- Dashboard: LLM cost per agent, per model, per day
- Cost queries are platform-wide. Dashboard shows cost totals. Token quotas can be set per agent or globally.

## Per-request usage and audit

With every request that uses an LLM, the user sees **token/LLM usage** (model, tokens in/out, latency, cost) in the response. Usage is captured in the LLM router (or caller), returned in the response envelope, and persisted **asynchronously** via the `astra:usage` stream to the `llm_usage` table and to `events` (type `LLMUsage`) for audit and metrics. No synchronous DB write on the hot path. See **docs/phase-history-usage-audit-design.md** and §18 Audit & Compliance.

**Token counts:** Backends (OpenAI, Anthropic, Gemini, Ollama, MLX) parse provider responses and return real `tokens_in`/`tokens_out`; the router builds `Usage` and the gRPC response and usage stream carry these values. The router cache stores and restores token usage on cache hit so cached responses still show non-zero counts. For MLX, the client tries `/v1/chat/completions` first and falls back to `/chat/completions` on 404 for compatibility with older mlx_lm.server versions.

---

# 24. Operational Playbooks

## Oncall Rotations

- Primary: Kernel SRE (scheduler, actors, tasks, messaging, state)
- Secondary: Agent Platform (workers, tools, memory, LLM)
- Escalation matrix in `docs/runbooks/`

## Incident Lifecycle

```
Detect → Triage → Contain → Remediate → Postmortem → Remediation Review
```

## Upgrade Plan

- Kernel upgrades are backward compatible (message contracts, schemas)
- Rolling upgrade with canary and DB migration windows
- Use blue/green deployment where possible

---

# 25. Acceptance Criteria & SLAs

## Functional Acceptance (MVP)

- [ ] Spawn and run a persistent agent (simple echo agent)
- [ ] Planner produces task DAGs from a goal
- [ ] Scheduler detects ready tasks and dispatches to workers
- [ ] Worker executes tasks and returns results (persisted in Postgres)
- [ ] Task state transitions emit events to `events` table
- [ ] Observability traces visible for each task execution
- [ ] Tool runtime can run sandboxed command and return artifact

## MVP Milestone Map

| MVP Criterion | Phase |
|---|---|
| Spawn and run persistent agent | Phase 1 |
| Planner produces task DAGs | Phase 4 |
| Scheduler dispatches to workers | Phase 1 |
| Worker executes in sandbox | Phase 2 |
| Event sourcing | Phase 1 |
| Observability traces | Phase 5 |
| Tool runtime in sandbox | Phase 2 |

## Production SLAs

| SLA | Target |
|---|---|
| Control plane API availability | 99.9% |
| API read response time (p99) | ≤ 10ms |
| Task scheduling latency (median) | ≤ 50ms |
| Task scheduling latency (P95) | ≤ 500ms |
| Task execution correctness | ≥ 99% pass rate |
| Worker failure detection | ≤ 30s |
| Event durability (persist to Postgres) | ≤ 1s |

---

# 26. Implementation Roadmap

## Agent platform hardening (migration 0024)

Single-tenant controls: **`agent_config_revisions`** (payload JSON: `system_prompt`, optional `config`); **`agents.active_config_revision`**; **`tool_definitions`** (`name`, `version`, `risk_tier`, `sandbox`, `description`, `metadata`); **`agents.drain_mode`**, **`max_concurrent_goals`**, **`daily_token_budget`**, **`priority`** (scheduler ordering), **`allowed_tools`** JSON array (`name@version` or `*`); **`chat_sessions.retention_days`**, **`memories.expires_at`**. Goal admission uses Redis **`agent:{id}:tokens:YYYY-MM-DD`** (O(1)); LLM inflight cap **`ASTRA_LLM_MAX_INFLIGHT`**. **Audit export** (`GET .../audit.ndjson`) may contain PII — restrict to superadmin, define retention in ops. **Forget agent** removes chat + memories for GDPR-style requests.

## Phase 0 — Prep (2 weeks) ✅ COMPLETE

**Goal:** Repository scaffolding and infrastructure.

- [x] Initialize Go module (`go mod init astra`)
- [x] Create directory structure per Section 4
- [x] Set up Postgres, Redis, Memcached, MinIO (docker-compose)
- [x] Run all migrations (Section 11) — 10 migrations (0000–0009), all idempotent
- [x] Generate proto stubs (`buf generate`) — buf.yaml, buf.gen.yaml, generated .pb.go in proto/kernel/, proto/tasks/
- [x] Set up CI pipeline (go vet, lint, test) — .github/workflows/ci.yml, .golangci.yml
- [x] Configure `.cursor/` agents (10), rules (9), skills (7), commands (6)
- [x] Phase history, LLM usage, and audit: schema (migration 0009), design doc, development-phase history in `docs/phase-history/`
- [x] Deploy script (scripts/deploy.sh) — native-first infra, idempotent migrations, builds and runs services
- [x] Implementation plans for phases 1–6 (docs/implementation-plans/), delegation memo, PRD currency rule

**Acceptance:** `docker compose up` starts all infra; `go build ./...` succeeds; migrations applied. **MET.**

## Phase 1 — Kernel MVP (8-10 weeks) ✅ COMPLETE

**Goal:** Actor runtime, state manager, message bus, task graph, scheduling loop, api-gateway, agent-service.

- [x] `internal/actors` — BaseActor, mailbox, supervision tree, circuit breaker ✅
- [x] `internal/kernel` — Kernel manager (Spawn, Send, Stop), metrics integration ✅
- [x] `internal/events` — Event store (Postgres insert, replay) ✅
- [x] `internal/messaging` — Redis Streams (publish, consume, ack, consumer groups) ✅
- [x] `internal/tasks` — Task model, state machine, Graph, transitions, CreateGraph, GetTask, CompleteTask, FailTask ✅
- [x] `internal/scheduler` — Ready-task detection, shard dispatch, 100ms tick loop ✅
- [x] `internal/agent` — AgentActor with CreateGoal → Plan → CreateTasks flow ✅
- [x] `internal/planner` — Stub: hardcoded two-task DAG (analyze + implement) ✅
- [x] `internal/kernelserver` — KernelService gRPC server (SpawnActor, SendMessage, QueryState, PublishEvent) ✅
- [x] `pkg/db` — Connection pool (pgx), migration runner ✅
- [x] `pkg/config` — Env config loader, PostgresDSN builder, AgentGRPCPort ✅
- [x] `pkg/logger` — Structured logging (slog JSON) ✅
- [x] `pkg/grpc` — Server creation, reflection, logging interceptor, ListenAndServe ✅
- [x] `pkg/metrics` — Prometheus registration (task latency, success, failure, actor count, queue depth) ✅
- [x] `cmd/api-gateway` — REST endpoints: POST /agents, POST /agents/{id}/goals, GET /tasks/{id}, GET /graphs/{id}, POST /tasks/{id}/complete, GET /health; gRPC proxy to agent-service and task-service ✅
- [x] `cmd/agent-service` — KernelService gRPC server on port 9091, agent factory, graceful shutdown ✅
- [x] `cmd/scheduler-service` — Scheduling loop, Postgres + Redis, signal handling ✅
- [x] `cmd/task-service` — TaskService gRPC server on port 9090, graceful shutdown ✅
- [x] `cmd/execution-worker` — Consumes Redis stream, transitions queued→scheduled→running→completed ✅
- [x] Unit tests: actors, kernel, tasks, planner, events, config, metrics ✅
- [x] Integration test: E2E spawn → goal → plan → schedule → worker complete → events (tests/integration/) ✅

**Stubs & Replacements:** Planner stub: hardcoded single-task DAG in `internal/planner`, replaced in Phase 4 with LLM-driven planning. Worker stub: simple pass-through in `cmd/execution-worker` that marks tasks complete, replaced in Phase 2 with real tool execution.

**Auth note:** api-gateway now enforces JWT validation and access-control checks in Phase 4. Health endpoints remain open for local/dev readiness probes.

**Acceptance:** Spawn agent → create goal → planner stubs DAG → scheduler dispatches → worker stubs complete task → events in Postgres → query state returns correct data.

## Phase 2 — Workers & Tool Runtime (6-8 weeks) ✅ COMPLETE

**Goal:** Execution workers, worker manager, tool sandboxes.

- [x] `internal/workers` — Worker struct with DB-backed Registry (Register, UpdateHeartbeat, MarkOffline, ListActive, FindStaleWorkers). NewWithDB constructor for DB-backed heartbeat. ✅
- [x] `internal/tools` — Runtime interface with DockerRuntime (real `docker run` with resource limits, network isolation, read-only rootfs) and NoopRuntime (phase 2 placeholder). ✅
- [x] `cmd/execution-worker` — Consumes from stream, claims task, sets worker_id, executes via tool runtime, CompleteTask/FailTask. Registry integration for heartbeat and shutdown. ✅
- [x] `cmd/worker-manager` — HTTP service (port 8082) with GET /workers and /health. Background loop marks stale workers offline (>30s). Re-queues orphaned running tasks and republishes to stream. ✅
- [x] `cmd/tool-runtime` — HTTP service (port 8083) with POST /execute (base64 I/O) and GET /health. Configurable Docker or Noop backend via TOOL_RUNTIME env. ✅
- [x] `cmd/browser-worker` — Specialized worker consuming from `astra:tasks:browser` stream. Uses NoopRuntime for Phase 2 (real Playwright deferred). Registers with ["browser"] capabilities. ✅
- [x] `migrations/0010_worker_task_tracking.sql` — Adds worker_id column to tasks table for tracking which worker claimed a task. ✅
- [x] `internal/tasks/store.go` — Added SetWorkerID, FindOrphanedRunningTasks, RequeueTask methods for worker failure recovery. ✅

**Acceptance:** Worker registers, claims task from Redis stream, executes via tool runtime (noop/Docker), sets worker_id, returns result persisted. Workers not heartbeating within 30s marked offline; orphaned tasks re-queued.

## Phase 3 — Memory & LLM Routing (6 weeks) ✅ COMPLETE

**Goal:** Memory service with pgvector, LLM router, Memcached caching. Hot-path reads compliant with 10ms SLA.

- [x] `internal/memory` — Write with optional embeddings, vector search (`embedding <=> query::vector`), GetByID, and embedding helpers (bytes<->float32). ✅
- [x] `internal/llm` — Router supports `Complete()`, model resolution, response caching, usage metrics, and stub backend fallback. ✅
- [x] `cmd/memory-service` — gRPC MemoryService with WriteMemory/SearchMemories/GetMemory on `MEMORY_GRPC_PORT` (9092). ✅
- [x] `cmd/llm-router` — gRPC LLMRouter Complete on `LLM_GRPC_PORT` (9093). ✅
- [x] `cmd/prompt-manager` — HTTP prompt manager with cache-aside Get/Save using `prompts` table (0011). ✅
- [x] Memcached integration for LLM/embedding caches (`llm:resp:*`, `embed:*`). ✅
- [x] Redis cache-aside for task reads (`task:{id}`, `graph:{id}`) via `internal/tasks/CachedStore`. ✅
- [x] Validation script updated with Phase 3 service and structural checks. ✅

**Caching note:** Phase 3 introduces cache-aside for major read paths; full production SLO enforcement and load-driven verification continue in Phase 5.

**Acceptance:** Agent memory write/search path implemented with pgvector, LLM router caches repeated prompts, prompt-manager persists templates, and task read hot-path can be served from cache.

## Phase 4 — Orchestration, Eval, Security (6-8 weeks) ✅ COMPLETE

**Goal:** Planner service, goal service, evaluation service, identity, access-control policy checks, approval gates, and async LLM usage audit.

- [x] `internal/planner` — LLM-backed DAG planning path with robust fallback graph and dependency synthesis. ✅
- [x] `internal/evaluation` — Default evaluator plus criteria-based checks (regex/substring). ✅
- [x] `cmd/planner-service` — HTTP planner service (`/plan`, `/health`) on `PLANNER_PORT` (8087). ✅
- [x] `cmd/goal-service` — Goal lifecycle API with `phase_runs` + `events` (`PhaseStarted`, `PhaseCompleted`, `PhaseFailed`) on `GOAL_SERVICE_PORT` (8088). ✅
- [x] `cmd/evaluation-service` — Evaluation API on `EVALUATION_PORT` (8089). ✅
- [x] `cmd/identity` — JWT issue/validate service (HS256) on `IDENTITY_PORT` (8085). ✅
- [x] `cmd/access-control` — Policy check service with approval workflows on `ACCESS_CONTROL_PORT` (8086). ✅
- [x] API gateway — JWT validation + access-control enforcement on protected routes. ✅
- [x] Tool execution approval gates — dangerous tools create `approval_requests` and return pending approval (0012). ✅
- [x] LLM usage async persistence — `astra:usage` stream consumer writes `llm_usage` and `events` (`LLMUsage`) without request-path DB writes. ✅

**Acceptance:** Goals can be submitted and planned, evaluator validates outputs, JWT+policy checks enforce protected API access, dangerous tool execution requires approval, and LLM usage is asynchronously audited.

## Phase 5 — Scale & Production Hardening (8 weeks) ✅ COMPLETE

**Goal:** Load testing, observability dashboards, runbooks, cost tracking.

- [x] Load test assets in `tests/load/` (k6 harness, scenarios, results template) ✅
- [x] Grafana dashboards (cluster overview, agent health, cost) under `deployments/grafana/dashboards/` ✅
- [x] Alerting rules in Prometheus under `deployments/prometheus/rules/astra-alerts.yaml` ✅
- [x] Runbooks in `docs/runbooks/` with index and alert mapping ✅
- [x] Cost tracking service (`cmd/cost-tracker`) and `internal/cost` aggregation utilities ✅
- [x] SLO enforcement alerts for read and scheduling latency in Prometheus rules ✅
- [x] Helm chart hardening: HPA + PDB templates and CI `helm template` validation ✅
- [x] Observability documentation in `docs/observability.md` and additional metrics definitions ✅

**Acceptance:** Phase 5 repository deliverables are implemented and validated in `scripts/validate.sh`. Full-scale load execution remains an environment runbook activity using `tests/load/` assets.

## Phase 6 — SDK & Applications (4-6 weeks initial) ✅ COMPLETE

**Goal:** Public Astra SDK, minimum viable sample applications.

- [x] SDK package with AgentContext, MemoryClient, ToolClient under `pkg/sdk` ✅
- [x] SimpleAgent example under `examples/simple-agent` ✅
- [x] SDK documentation and examples (`pkg/sdk/README.md`, `examples/`) ✅
- [x] Additional example app `examples/echo-agent` ✅

**Minimum scope (4-6 weeks):** AgentContext interface, MemoryClient interface, SimpleAgent example, SDK documentation are implemented. Ongoing post-MVP SDK roadmap still includes richer sample agents (autonomous developer, research), Python/TS bindings, and community docs.

## Phase 7 — Security Compliance & Production Auth (6-8 weeks) ✅ COMPLETE

**Goal:** Enforce production-ready transport security and runtime secret management for all core services.

- [x] TLS-aware gRPC server/client wiring through `pkg/grpc` and all gRPC services ✅
- [x] TLS-aware HTTP listener/client wiring through `pkg/httpx` and service mains ✅
- [x] Vault-backed secret overlay via `pkg/secrets` + `pkg/config` ✅
- [x] TLS/Vault operational runbooks in `docs/runbooks/` ✅
- [x] Phase 7 validation checks in `scripts/validate.sh` ✅

**Acceptance:** Security transport and secret-loading foundations are implemented in code paths used by services, with local-dev fallback still supported when TLS/Vault are disabled.

## Phase 8 — Platform Visibility Dashboard (2-3 weeks) ✅ COMPLETE

**Goal:** Provide a utilitarian operations dashboard with complete live visibility into platform runtime state.

- [x] Embedded dashboard UI served by api-gateway at `/dashboard/` ✅
- [x] Snapshot API at `/api/dashboard/snapshot` with services/workers/approvals/cost/logs/pids ✅
- [x] Snapshot enriched with agents (agent_count, agents list) from agent-service QueryState ✅
- [x] Summary stats include Agents count; Agents doughnut chart (by status) next to Service Health ✅
- [x] Agents table with Previous/Next pagination to scroll through all agents ✅
- [x] Recent Goals rows clickable; goal detail modal shows full goal text, actions (tasks), and failure logs for failed tasks ✅
- [x] Goal-service `GET /goals/{id}/details` returns goal + tasks (task store `ListTasksByGoalID`); gateway `GET /api/dashboard/goals/{id}` proxies for dashboard ✅
- [x] Auto-refreshing frontend with status and latency indicators ✅
- [x] Dashboard validation coverage in `scripts/validate.sh` ✅

**Acceptance:** Operators can open one dashboard and inspect service health, workers, **agents** (count, chart, paginated list), pending approvals, cost trends, process state, and recent logs without shell access. Clicking a goal opens a modal with full goal description, task list (actions), and failure logs for failed tasks.

## Phase 9 — Agent Profile & Context Management (3-4 weeks)

**Goal:** Give agents a structured identity (system prompt, config) and attach contextual documents (rules, skills, context docs) that propagate through the planning and execution pipeline.

- [ ] `migrations/0013_agent_profile_and_documents.sql` — Add `system_prompt` to agents, create `agent_documents` table with indexes and constraints
- [ ] `internal/agentdocs/store.go` — Document CRUD (Create, List, Delete), profile read/write, Redis cache-aside for `agent:profile:{id}` and `agent:docs:{id}` (5min TTL)
- [ ] `internal/agentdocs/context.go` — Context assembly: merge system_prompt + priority-sorted rules + skills + context_docs into a single `AgentContext` struct
- [ ] `cmd/api-gateway` — New REST endpoints: `PATCH /agents/{id}`, `GET /agents/{id}/profile`, `POST /agents/{id}/documents`, `GET /agents/{id}/documents`, `DELETE /agents/{id}/documents/{doc_id}`
- [ ] `cmd/goal-service` — Accept optional `documents` array in `POST /goals`; persist as goal-scoped documents; assemble and pass `agent_context` to planner
- [ ] `cmd/planner-service` — Embed `agent_context` into each task payload during DAG generation
- [ ] `cmd/execution-worker` — Extract `agent_context` from task payload and include in LLM prompt construction
- [ ] Redis caching — `agent:profile:{id}` (Hash, 5min TTL), `agent:docs:{id}` (JSON string, 5min TTL); invalidate on write
- [ ] Large document support — Documents exceeding size threshold stored in MinIO/S3 with URI reference
- [ ] Unit tests for `internal/agentdocs` (store, context assembly, cache invalidation)
- [ ] Integration test: create agent with profile → attach documents → submit goal with inline docs → verify context propagates to worker task payload
- [ ] Phase 9 validation checks in `scripts/validate.sh`

**Acceptance:** Agent profiles and documents can be created and queried via REST API. Goal submission with inline documents works. The full agent context (system_prompt + rules + skills + context_docs) is assembled by goal-service, passed through the planner into task payloads, and available to execution workers for LLM prompt construction. Profile and document reads served from Redis cache within 10ms SLA.

## Phase 10: Chat Agents (WebSocket streaming)

- [x] Migration 0016: chat_sessions, chat_messages, agents.chat_capable
- [x] Session REST API (POST/GET /chat/sessions, GET /chat/sessions/{id})
- [x] WebSocket /chat/ws with JWT auth, session validation
- [x] Streaming chat loop (LLM → chunks → message_end → done)
- [x] Tool invocation from chat (tool_call → tool-runtime → tool_result)
- [x] Memory context integration (optional)
- [x] Rate limits and token caps (per-session)
- [x] Dashboard chat UI (session list, message panel, new chat)
- [x] Config: CHAT_ENABLED, CHAT_MAX_MSG_LENGTH, CHAT_RATE_LIMIT, CHAT_TOKEN_CAP

**Acceptance:** Users can create chat sessions, connect via WebSocket, send messages and receive streaming responses, with optional tool invocation and memory context.

## Phase 11 — Multi-Tenancy (8-12 weeks)

**Goal:** Convert Astra from single-tenant to a privacy-focused multi-tenant platform with organizations, teams, users, roles, and tiered agent visibility.

### Sub-phase 11.0: PRD update
- [x] Update PRD (this document) with full multi-tenancy architecture specification ✅

### Sub-phase 11.1: Database schema
- [ ] `migrations/0018_multi_tenant.sql` — New tables: `users`, `organizations`, `org_memberships`, `teams`, `team_memberships`, `agent_collaborators`, `agent_admins`. ALTER: `agents`, `goals`, `tasks`, `workers`, `events`, `memories`, `llm_usage`, `approval_requests`, `chat_sessions` with `org_id` and related columns. Indexes and triggers.

### Sub-phase 11.2: Identity service overhaul
- [ ] `internal/identity/` — User CRUD, login (bcrypt), password reset
- [ ] `cmd/identity/main.go` — New endpoints: `POST /users/login`, `POST /users`, `GET /users`, `GET /users/{id}`, `PATCH /users/{id}`. Enriched JWT claims (user_id, org_id, org_role, team_ids, is_super_admin).

### Sub-phase 11.3: RBAC engine
- [ ] `internal/rbac/rbac.go` — Role-based authorization engine
- [ ] `internal/rbac/visibility.go` — Agent visibility logic (`CanAccessAgent`)
- [ ] `internal/rbac/middleware.go` — HTTP/gRPC middleware for route guards and org isolation

### Sub-phase 11.4: Org, team, and user management API
- [ ] `internal/orgs/` — Org and team CRUD, membership management
- [ ] Super-admin endpoints: `/superadmin/api/orgs/*`, `/superadmin/api/users/*` (15+ endpoints for platform-wide user management)
- [ ] Org-level endpoints: `/org/api/teams/*`, `/org/api/members/*`

### Sub-phase 11.5: Agent ownership, visibility, and collaboration
- [ ] Agent creation with `org_id`, `owner_id`, `team_id`, `visibility`
- [ ] Agent collaborator and agent-admin endpoints
- [ ] `CanAccessAgent` enforcement on all agent queries

### Sub-phase 11.6: Org-scoped data isolation
- [ ] All service queries (goal-service, task-service, agent-service, worker-manager, cost-tracker, memory-service) add `WHERE org_id = $orgID`
- [ ] Approval routing to agent admins (fallback to org admins)
- [ ] gRPC metadata propagation: `x-user-id`, `x-org-id`, `x-org-role`, `x-team-ids`, `x-is-super-admin`

### Sub-phase 11.7: URL restructure and dashboards
- [ ] Rename `/dashboard/` to `/superadmin/dashboard/`
- [ ] Build org home (`/org/`) and org admin dashboard (`/org/dashboard`)
- [ ] Build login page (`/login`)
- [ ] Super-admin dashboard: org management, platform-wide user management (paginated table, detail modal, actions)

### Sub-phase 11.8: Gateway middleware overhaul
- [ ] JWT-authenticated route guards (super_admin, org_admin, org_member)
- [ ] Context propagation to downstream services

### Sub-phase 11.9: Workspace isolation
- [ ] `WORKSPACE_ROOT/{org_slug}/{goal_id}/` for org agents
- [ ] `WORKSPACE_ROOT/_global/{goal_id}/` for global agents

### Sub-phase 11.10: Validation and deployment
- [ ] Update `scripts/validate.sh` with multi-tenant checks
- [ ] Update `scripts/deploy.sh` with super-admin seeding
- [ ] Update `.env` / `.env.example` with `ASTRA_SUPER_ADMIN_EMAIL`, `ASTRA_SUPER_ADMIN_PASSWORD`

**Acceptance:** Organizations, teams, and users can be managed. Agents have tiered visibility (global/public/team/private) with collaborator support. Org data is strictly isolated. Super-admins see platform-wide metrics (redacted) and manage all users. Org-admins see 100% of their org's data. Private agents are invisible to non-collaborators. Approval routing goes to agent admins.

## Phase 12 (future): Slack integration

**Goal:** Connect Astra chat agents to Slack so users can interact with agents via Slack DMs and channels.

**Dependencies:** Phase 10 (Chat), Phase 11 (Multi-tenancy). Requires internal or org-scoped chat append-message API callable by adapter/worker.

| Order | Deliverable |
|-------|-------------|
| 1 | DB migration: `slack_workspaces`, `slack_channel_bindings`, `slack_user_mappings` (optional: `slack_sessions`). |
| 2 | Internal/org-scoped chat append-message path callable with service or org-scoped auth (if not already). |
| 3 | **slack-adapter** service: Slack Request URL handler; verify signing secret; resolve org/agent/user; enqueue to Redis stream `astra:slack:incoming`; respond 200 within 3s. |
| 4 | Slack worker: consume stream; load bot token from Vault; create/resume session; call chat API; post reply to Slack; retries and rate limits. |
| 5 | **Platform Slack secrets UI:** Super-admin dashboard form to enter and save Slack app Signing Secret, Client ID, Client Secret, OAuth Redirect URL (stored encrypted in DB; env fallback). |
| 6 | OAuth flow: org dashboard "Connect Slack" → Slack OAuth (using platform config) → store workspace + bot token in Vault; insert/update `slack_workspaces`. |
| 7 | Org settings: default agent for Slack; per-channel agent binding; Slack user → Astra user mapping. |
| 8 | Optional: slash command (e.g. `/astra-goal`) → goal submit via goal-service. |
| 9 | Internal API: **POST /internal/slack/post** for proactive Slack messages; optional **notification_channel_id** per workspace. |

**Acceptance:** Org admin connects a Slack workspace to an org; users in that workspace can message the bot in DMs or channels and receive agent replies. Replies are posted asynchronously; adapter responds to Slack within 3s. Design: **docs/slack-integration-design.md**.

## Phase 12b (backlog): Multi-channel expansion — deferred

| Item | Status |
|------|--------|
| Slack channels / DMs (full worker + OAuth parity) | Partial; see Phase 12 |
| Microsoft Teams, email bots, additional messaging surfaces | Not started |
| Unified multi-channel session routing (one agent, many surfaces) | Not started |

**Scope when picked up:** Extend beyond single dashboard chat + ingest: channel registry, per-channel agent binding, shared session model. Single-tenant chat + external ingest remain until this phase.

---

# 27. Build Order & Dependency Graph

Implementation must follow this dependency order. Packages listed earlier must be completed before packages that depend on them.

```
Layer 0 (foundation — no internal deps):
  pkg/config
  pkg/logger
  pkg/metrics
  pkg/models
  pkg/db
  pkg/grpc
  pkg/otel

Layer 1 (kernel primitives — depend on pkg/*):
  internal/actors     (depends on: pkg/logger, pkg/metrics)
  internal/messaging  (depends on: pkg/config, pkg/logger)
  internal/events     (depends on: pkg/db, pkg/logger)

Layer 2 (kernel engine — depend on Layer 1):
  internal/kernel     (depends on: internal/actors, internal/messaging)
  internal/tasks      (depends on: pkg/db, internal/events)
  internal/scheduler  (depends on: internal/tasks, internal/messaging, pkg/db)

Layer 3 (services — depend on Layer 2):
  internal/llm        (depends on: pkg/config)
  internal/agent      (depends on: internal/kernel, internal/tasks)
  internal/planner    (depends on: internal/tasks, internal/llm)
  internal/memory     (depends on: pkg/db, internal/llm)
  internal/workers    (depends on: internal/messaging, internal/tasks)
  internal/tools      (depends on: pkg/config)
  internal/evaluation (depends on: internal/tasks)
  internal/agentdocs  (depends on: pkg/db, pkg/config; used by: cmd/api-gateway, cmd/goal-service)
  internal/identity   (depends on: pkg/db, pkg/config; used by: cmd/identity)
  internal/orgs       (depends on: pkg/db, pkg/config; used by: cmd/api-gateway)
  internal/rbac       (depends on: pkg/config; used by: cmd/api-gateway, cmd/access-control)

Layer 4 (entrypoints — depend on Layer 3):
  cmd/api-gateway
  cmd/agent-service
  cmd/scheduler-service
  cmd/task-service
  cmd/execution-worker
  cmd/worker-manager
  cmd/tool-runtime
  cmd/memory-service
  cmd/llm-router
  cmd/planner-service
  cmd/evaluation-service
  cmd/identity
  cmd/access-control
  cmd/prompt-manager
  cmd/browser-worker
  cmd/goal-service
  cmd/slack-adapter   (optional; depends on chat, org, Redis)
```

---

*End of specification. This document is the single source of truth for building Astra.*
