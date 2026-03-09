# ASTRA — The Autonomous Agent Operating System

**Engineering Specification v2.0**

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
19. [Deployment Architecture & Scaling](#19-deployment-architecture--scaling) (includes [Platform & Hardware Acceleration](#platform--hardware-acceleration))
20. [Failure Modes, Recovery & Runbooks](#20-failure-modes-recovery--runbooks)
21. [CI/CD, Testing & Release Plan](#21-cicd-testing--release-plan)
22. [Cost Management & LLM Routing](#22-cost-management--llm-routing)
23. [Operational Playbooks](#23-operational-playbooks)
24. [Acceptance Criteria & SLAs](#24-acceptance-criteria--slas)
25. [Implementation Roadmap](#25-implementation-roadmap)
26. [Build Order & Dependency Graph](#26-build-order--dependency-graph)

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
    /evaluation                # evaluators, test harness integration
    /events                    # event store, event replay, event sourcing
    /messaging                 # Redis Streams clients, consumer groups, backoff, ack
    /llm                       # LLM router logic, model selection, caching
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
| `internal/llm` | Service | LLM model routing, cost-based selection, response caching |
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
| 1 | `api-gateway` | REST/gRPC gateway, auth middleware, rate limiting, versioning | control-plane |
| 2 | `identity` | User/service auth, JWT tokens, audit log | control-plane |
| 3 | `access-control` | RBAC, OPA policy enforcement, per-agent permission scopes | control-plane |
| 4 | `agent-service` | Agent lifecycle (spawn/stop/inspect), actor supervisor integration | kernel |
| 5 | `goal-service` | Goal ingestion, validation, routing to planner | kernel |
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
Fields: `request_id`, `agent_id`, `task_id`, `model`, `tokens_in`, `tokens_out`, `latency_ms`, `cost_dollars`, `timestamp`. Consumer writes to `llm_usage` and appends to `events` with type `LLMUsage`. Keeps API under 10 ms (no synchronous DB write on LLM response path).

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

- `identity` service handles users and service accounts
- JWT tokens for external API authentication (short-lived, signed)
- mTLS for all service-to-service communication

## Authorization

- `access-control` service uses OPA policies with RBAC
- Policies stored in Git and versioned
- Agents run with minimum privileges; per-agent permission scopes

## Secrets

- Secrets injected at runtime from Vault
- Never persisted in code, logs, events, or artifact metadata
- Tool sandboxes receive secrets via ephemeral volumes only

## Tool & Action Governance

- Dangerous actions (delete infra, change prod, data deletion) require approval gates (human-in-the-loop)
- `policy-engine` intercepts tool execution events and allows/denies based on policy

## Audit & Compliance

- **Immutable event log** — The `events` table is the canonical audit log. It includes task/agent lifecycle events and, for phase/usage audit: `PhaseStarted`, `PhaseCompleted`, `PhaseFailed`, `PhaseSummary`, and `LLMUsage` (per-request model, tokens in/out, latency, cost). Index on `created_at` supports time-range audit queries.
- **Phase/build history** — Each phase run is recorded in `phase_runs` (and optionally in `phase_summaries` for semantic search) and as a human-readable file per phase; phase lifecycle events are appended to `events`. Development-phase history (what was built in each implementation phase) is maintained in **docs/phase-history/** (e.g. phase-0.md).
- **Per-request LLM usage** — Visible to the user in the response; persisted asynchronously to `llm_usage` and to `events` as `LLMUsage` for analytics and audit.
- Artifact immutability (once written, cannot be modified).
- All state transitions logged with actor_id and timestamp.

Full design: **docs/phase-history-usage-audit-design.md**.

---

# 19. Deployment Architecture & Scaling

## Kubernetes Namespaces

| Namespace | Services |
|---|---|
| `control-plane` | api-gateway, identity, access-control |
| `kernel` | scheduler-service, task-service, agent-service, goal-service |
| `workers` | execution-worker, browser-worker, tool-runtime, worker-manager, llm-router, prompt-manager, evaluation-service |
| `infrastructure` | postgres (primary + replicas), redis (cluster), memcached (clustered), minio |
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

# 20. Failure Modes, Recovery & Runbooks

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

# 21. CI/CD, Testing & Release Plan

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

# 22. Cost Management & LLM Routing

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

## Per-request usage and audit

With every request that uses an LLM, the user sees **token/LLM usage** (model, tokens in/out, latency, cost) in the response. Usage is captured in the LLM router (or caller), returned in the response envelope, and persisted **asynchronously** via the `astra:usage` stream to the `llm_usage` table and to `events` (type `LLMUsage`) for audit and metrics. No synchronous DB write on the hot path. See **docs/phase-history-usage-audit-design.md** and §18 Audit & Compliance.

---

# 23. Operational Playbooks

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

# 24. Acceptance Criteria & SLAs

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

# 25. Implementation Roadmap

## Phase 0 — Prep (2 weeks)

**Goal:** Repository scaffolding and infrastructure.

- [ ] Initialize Go module (`go mod init astra`)
- [ ] Create directory structure per Section 4
- [ ] Set up Postgres, Redis, Memcached, MinIO (docker-compose)
- [ ] Run all migrations (Section 11)
- [ ] Generate proto stubs (`buf generate`)
- [ ] Set up CI pipeline (go vet, lint, test)
- [ ] Configure `.cursor/` agents, rules, skills
- [ ] Phase history, LLM usage, and audit: schema (migration 0009: `phase_runs`, `phase_summaries`, `llm_usage`, `events` index), design doc (`docs/phase-history-usage-audit-design.md`), and development-phase history in `docs/phase-history/` (e.g. phase-0.md)

**Acceptance:** `docker compose up` starts all infra; `go build ./...` succeeds; migrations applied.

## Phase 1 — Kernel MVP (8-10 weeks)

**Goal:** Actor runtime, state manager, message bus, task graph, scheduling loop, api-gateway, agent-service.

- [ ] `internal/actors` — BaseActor, mailbox, supervision tree
- [ ] `internal/kernel` — Kernel manager (Spawn, Send)
- [ ] `internal/events` — Event store (Postgres insert, replay)
- [ ] `internal/messaging` — Redis Streams (publish, consume, ack)
- [ ] `internal/tasks` — Task model, state machine, Graph, transitions
- [ ] `internal/scheduler` — Ready-task detection, shard dispatch
- [ ] `internal/agent` — AgentActor, agent lifecycle
- [ ] `internal/planner` — Stub: hardcoded single-task DAG (replaced in Phase 4)
- [ ] `pkg/db` — Connection pool, migration runner
- [ ] `pkg/config` — Env/Vault config loader
- [ ] `pkg/logger` — Structured logging
- [ ] `pkg/grpc` — Server/client helpers, interceptors
- [ ] `pkg/metrics` — Prometheus registration
- [ ] `cmd/api-gateway` — REST health + gRPC proxy
- [ ] `cmd/agent-service` — Agent CRUD via gRPC
- [ ] `cmd/scheduler-service` — Scheduling loop
- [ ] `cmd/task-service` — Task CRUD via gRPC
- [ ] `cmd/execution-worker` — Stub: pass-through marks tasks complete (replaced in Phase 2)
- [ ] Unit tests for all packages, integration tests with testcontainers

**Stubs & Replacements:** Planner stub: hardcoded single-task DAG in `internal/planner`, replaced in Phase 4 with LLM-driven planning. Worker stub: simple pass-through in `cmd/execution-worker` that marks tasks complete, replaced in Phase 2 with real tool execution.

**Auth note:** api-gateway runs with no auth in dev/test. A placeholder middleware accepts all requests. Full S2 (JWT + mTLS) compliance is achieved in Phase 4.

**Acceptance:** Spawn agent → create goal → planner stubs DAG → scheduler dispatches → worker stubs complete task → events in Postgres → query state returns correct data.

## Phase 2 — Workers & Tool Runtime (6-8 weeks)

**Goal:** Execution workers, worker manager, tool sandboxes.

- [ ] `internal/workers` — Worker registration, heartbeat, task claiming
- [ ] `internal/tools` — Sandbox lifecycle (Docker first, WASM later)
- [ ] `cmd/execution-worker` — General worker runtime
- [ ] `cmd/worker-manager` — Worker health, scaling hints
- [ ] `cmd/tool-runtime` — Tool sandbox controller
- [ ] `cmd/browser-worker` — Headless browser worker (Playwright)

**Acceptance:** Worker registers, claims task from Redis stream, executes in Docker sandbox, returns artifact, result persisted.

## Phase 3 — Memory & LLM Routing (6 weeks)

**Goal:** Memory service with pgvector, LLM router, Memcached caching. Hot-path reads compliant with 10ms SLA.

- [ ] `internal/memory` — Write, search (pgvector), embedding pipeline
- [ ] `internal/llm` — Router logic, model selection, response caching
- [ ] `cmd/memory-service` — Memory CRUD + search API
- [ ] `cmd/llm-router` — Model routing service
- [ ] `cmd/prompt-manager` — Prompt template management
- [ ] Memcached integration for LLM/embedding/tool caches
- [ ] Redis cache-aside for actor state (`actor:state:<id>`) and task lookups
- [ ] Memcached for hot-path API reads (task status, agent state)

**Caching note:** Phases 1-2 may exceed 10ms on reads (acceptable for dev). Phase 3 brings hot-path reads into SLA compliance.

**Acceptance:** Agent writes memory → search returns semantically relevant results. LLM router selects model based on task type. Repeated prompts served from cache. API reads serve from Redis/Memcached; p99 ≤ 10ms.

## Phase 4 — Orchestration, Eval, Security (6-8 weeks)

**Goal:** Planner service, goal service, evaluation service, OPA integration, approval gates.

- [ ] `internal/planner` — Goal → DAG conversion using LLM
- [ ] `internal/evaluation` — Result validators, auto-evaluators
- [ ] `cmd/planner-service` — Planner API
- [ ] `cmd/goal-service` — Goal ingestion, validation, routing to planner-service
- [ ] `cmd/evaluation-service` — Evaluation API
- [ ] `cmd/identity` — JWT token issuance
- [ ] `cmd/access-control` — OPA policy enforcement
- [ ] Tool execution approval gates

**Acceptance:** Goal submitted via goal-service → planner generates real DAG → scheduler executes → evaluator validates → security policies enforced.

## Phase 5 — Scale & Production Hardening (8 weeks)

**Goal:** Load testing, observability dashboards, runbooks, cost tracking.

- [ ] Load tests (target: 10k agents, 1M tasks)
- [ ] Grafana dashboards (cluster overview, agent health, cost)
- [ ] Alerting rules in Prometheus
- [ ] Runbooks in `docs/runbooks/`
- [ ] Cost tracking service
- [ ] SLO enforcement (10ms reads, 50ms scheduling)
- [ ] Helm chart hardening (HPA, PDB, resource limits)

**Acceptance:** System handles target load within SLAs. Dashboards operational. Runbooks tested.

## Phase 6 — SDK & Applications (4-6 weeks initial)

**Goal:** Public Astra SDK, minimum viable sample applications.

- [ ] SDK package with AgentContext, MemoryClient, ToolClient
- [ ] SimpleAgent example
- [ ] SDK documentation and examples

**Minimum scope (4-6 weeks):** AgentContext interface, MemoryClient interface, SimpleAgent example, SDK documentation. After initial SDK, ongoing work includes additional sample agents (autonomous developer, research), Python/TS bindings, community docs.

---

# 26. Build Order & Dependency Graph

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
```

---

*End of specification. This document is the single source of truth for building Astra.*
