# Phase 1 — Kernel MVP — Implementation Plan

**Depends on:** Phase 0 complete (PRD §25). Acceptance: `docker compose up` starts infra; `go build ./...` succeeds; migrations applied.

**PRD references:** §4 Monorepo, §5 Kernel, §6 Actors, §7 Task Graph, §8 Scheduler, §9 Services, §10 Protobuf, §11 DB, §12 Events/Messaging, §25 Phase 1, §26 Build Order.

---

## 1. Phase goal

Deliver a working kernel MVP: actor runtime, state manager, message bus, task graph with state machine, scheduling loop, api-gateway, agent-service, scheduler-service, task-service, and a stub execution-worker — so that spawning an agent, creating a goal, stubbing a single-task DAG, dispatching to the worker, and completing the task all flow end-to-end with events in Postgres.

---

## 2. Dependencies

- **Phase 0** signed off: infra up, migrations applied, proto generated, CI green, `go build ./...` passes.
- **Build order (PRD §26):** Layer 0 (pkg/*) before Layer 1; Layer 1 before Layer 2; Layer 2 before Layer 3; Layer 3 before cmd/*.

---

## 3. Work packages

### WP1.1 — Shared packages (pkg/)

**Description:** Implement or finalize `pkg/db`, `pkg/config`, `pkg/logger`, `pkg/grpc`, `pkg/metrics`, `pkg/models` so all kernel and service code can depend on them. DB pool and migration runner must apply migrations from `migrations/`; config loads from env (and optionally Vault); logger is structured (slog or zerolog); gRPC helpers provide server/client setup and interceptors; metrics register Prometheus counters/gauges/histograms per PRD §17.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Implement `pkg/db`: connection pool (pgx), migration runner that runs `migrations/*.sql` in order, idempotent. | Go Engineer | `pkg/db` package |
| 2 | Implement `pkg/config`: load from env vars; document Vault extension point (no Vault impl required in Phase 1). | Go Engineer | `pkg/config` package |
| 3 | Implement `pkg/logger`: structured logging (slog or zerolog), configurable level. | Go Engineer | `pkg/logger` package |
| 4 | Implement `pkg/grpc`: server/client helpers, optional interceptors (logging, tracing placeholder). | Go Engineer | `pkg/grpc` package |
| 5 | Implement `pkg/metrics`: Prometheus registration, helpers for counters/gauges/histograms used in Phase 1. | Go Engineer | `pkg/metrics` package |
| 6 | Implement `pkg/models`: shared types (e.g. UUID handling, status enums) used by kernel and tasks. | Go Engineer | `pkg/models` package |
| 7 | Unit tests for each pkg. | Go Engineer | Tests in pkg/* |
| 8 | CI: ensure `go build ./...` and `go test ./...` include pkg. | DevOps | CI green |

**Deliverables:** All `pkg/*` packages implemented and tested; importable by internal packages.

**Acceptance criteria:** Any internal package can import pkg/db, pkg/config, pkg/logger, pkg/grpc, pkg/metrics, pkg/models; migration runner applies all files in `migrations/` without error; CI passes.

---

### WP1.2 — Event store (internal/events)

**Description:** Implement `internal/events`: append-only event store in Postgres. Insert events (event_type, actor_id, payload JSONB, created_at); support replay by entity or time range. Must align with `events` table (migrations/0006_events.sql, 0007_indexes.sql). Used by task state machine for event sourcing (PRD §7, §12).

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Implement event store: Insert(ctx, event_type, actor_id, payload) writing to `events` table. | Go Engineer | internal/events/store.go (or equivalent) |
| 2 | Implement Replay(ctx, filters) for querying events by actor_id and/or event_type and optional time range. | Go Engineer | Replay API |
| 3 | Use pkg/db for DB access; pkg/logger for logging. | Go Engineer | No new deps |
| 4 | Unit tests with testcontainers or in-memory/sqlite if acceptable; else integration test against real Postgres. | QA / Go Engineer | Tests |

**Deliverables:** `internal/events` package with Insert and Replay; tests.

**Acceptance criteria:** Insert appends to `events`; Replay returns events in order; no synchronous dependency on Redis in this package.

---

### WP1.3 — Messaging / Redis Streams (internal/messaging)

**Description:** Implement `internal/messaging`: Redis Streams client — Publish(stream, fields), Consume(stream, group, consumer, handler) with consumer groups, XACK after successful handling. Support the stream names from PRD §12 (e.g. `astra:tasks:shard:<n>`, `astra:events`). Connection from pkg/config (Redis addr). Reference implementation in PRD §12.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Implement Bus struct: New(addr string or from config), Publish(ctx, stream, fields map[string]interface{}). | Go Engineer | internal/messaging/bus.go |
| 2 | Implement Consume with XReadGroup, consumer group creation (XGroupCreateMkStream), ack on success, block timeout. | Go Engineer | Consume API |
| 3 | Use redis/go-redis; document stream key conventions (astra:tasks:shard:0 for Phase 1 single shard). | Go Engineer | Docs or code comments |
| 4 | Unit or integration tests: publish and consume round-trip (may use testcontainers Redis). | QA / Go Engineer | Tests |

**Deliverables:** `internal/messaging` package; tests.

**Acceptance criteria:** Publish writes to a stream; Consume reads and acks; scheduler and worker can use the same bus.

---

### WP1.4 — Actor runtime (internal/actors)

**Description:** Implement `internal/actors`: Actor interface (ID, Receive, Stop), BaseActor with mailbox (chan Message), Start(handler), non-blocking Receive returning ErrMailboxFull when full. Message type per PRD §6 (ID, Type, Source, Target, Payload, Meta, Timestamp). Optional: supervision tree types (RestartImmediate, RestartBackoff, Escalate, Terminate) with a minimal supervisor for Phase 1 — at least one concrete actor runnable.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Define Actor interface and Message type per PRD §6. | Go Engineer | internal/actors/actor.go |
| 2 | Implement BaseActor: mailbox (buffered channel), Start(handler), Receive (non-blocking), Stop (close stop chan, wait). | Go Engineer | internal/actors/actor.go or base.go |
| 3 | Add ErrMailboxFull and document capacity (e.g. 1024). | Go Engineer | Same |
| 4 | Optional Phase 1: minimal Supervisor that can spawn and watch one child (restart on panic or error). Defer full supervision tree to later if scope is large. | Go Engineer | internal/actors/supervisor.go (stub acceptable) |
| 5 | Unit tests: actor receives message, handler runs, Stop terminates. | Go Engineer | Tests |

**Deliverables:** `internal/actors` package; tests.

**Acceptance criteria:** Kernel can hold a map of Actor by ID and call Receive to deliver messages; BaseActor runs handler in a goroutine and stops cleanly.

---

### WP1.5 — Kernel manager (internal/kernel)

**Description:** Implement `internal/kernel`: Kernel struct with actor registry (map or sync.Map), Spawn(actor), Send(ctx, targetID, msg) routing to actor’s Receive. No gRPC yet — in-process API only. Kernel is the single point that owns actor set and message delivery (PRD §5).

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Implement Kernel: New(), Spawn(a Actor), Send(ctx, targetID string, msg Message) error. | Go Engineer | internal/kernel/kernel.go |
| 2 | Thread-safe registry (mutex or sync.Map). Return error if target not found. | Go Engineer | Same |
| 3 | Integrate with internal/actors (Actor interface, Message). | Go Engineer | Same |
| 4 | Unit tests: Spawn two actors, Send from one to another via Kernel. | Go Engineer | Tests |

**Deliverables:** `internal/kernel` package; tests.

**Acceptance criteria:** Spawn registers actor; Send delivers message to correct actor; unknown target returns error.

---

### WP1.6 — Task model and state machine (internal/tasks)

**Description:** Implement `internal/tasks`: Task and Graph types per PRD §7; Status enum (created, pending, queued, scheduled, running, completed, failed); Store with CreateTask, Transition(ctx, taskID, from, to, payload) in a transaction (UPDATE tasks SET status, INSERT into events). Dependency model: task_dependencies table; “ready” = all dependencies completed. Persist to Postgres using migrations 0001, 0002, 0006, 0008.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Define Task, Graph, Dependency, Status in Go; align with DB columns (graph_id, goal_id, agent_id, type, status, payload, result, priority, retries, max_retries). | Go Engineer | internal/tasks/task.go |
| 2 | Implement Store: CreateTask (insert task + dependency rows), GetTask, GetGraph. | Go Engineer | internal/tasks/store.go |
| 3 | Implement Transition(ctx, taskID, from, to, payload): single transaction that UPDATEs tasks and INSERTs into events (event_type e.g. TaskCompleted). Use pkg/db and internal/events or direct INSERT into events. | Go Engineer | Same |
| 4 | Implement “ready” query: tasks with status=pending and all dependencies completed (for scheduler use). Can be method on Store or separate. | Go Engineer | Same |
| 5 | Enforce valid transitions per PRD §7 table (e.g. created→pending, pending→queued, running→completed/failed). | Go Engineer | Same |
| 6 | Unit/integration tests: create graph with two tasks and one dependency, transition first to completed, then second becomes ready. | QA / Go Engineer | Tests |

**Deliverables:** `internal/tasks` package; tests.

**Acceptance criteria:** CreateTask and Transition persist correctly; events table gets one row per transition; ready detection matches PRD.

---

### WP1.7 — Scheduler loop (internal/scheduler)

**Description:** Implement `internal/scheduler`: find ready tasks (SQL with FOR UPDATE SKIP LOCKED per PRD §8), mark them queued in same transaction, push to Redis stream `astra:tasks:shard:0` (single shard for Phase 1). Run loop (tick every 100ms or configurable). Use internal/tasks.Store and internal/messaging.Bus.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Implement Scheduler struct: New(db, bus), Run(ctx) with ticker loop, tick(ctx) that finds ready tasks, marks queued, publishes to stream. | Go Engineer | internal/scheduler/scheduler.go |
| 2 | Ready query: SELECT t.id FROM tasks t WHERE status='pending' AND NOT EXISTS (deps not completed) FOR UPDATE SKIP LOCKED LIMIT N. | Go Engineer | Same |
| 3 | In same transaction (or immediately after): UPDATE tasks SET status='queued' WHERE id IN (...). Then Publish to astra:tasks:shard:0 with task_id, graph_id, agent_id, etc. per PRD §12. | Go Engineer | Same |
| 4 | Use pkg/logger for errors; optional pkg/metrics for tasks_enqueued. | Go Engineer | Same |
| 5 | Unit/integration tests: mock or real DB and Redis — create pending task with deps met, run tick, assert task status queued and message in stream. | QA / Go Engineer | Tests |

**Deliverables:** `internal/scheduler` package; tests.

**Acceptance criteria:** Ready tasks move to queued and appear on Redis stream; no double-dispatch (SKIP LOCKED and single transaction).

---

### WP1.8 — Planner stub (internal/planner)

**Description:** Implement `internal/planner` as a stub: given a goal (goal_id, agent_id), produce a single-task DAG (one task, no dependencies). Return Graph that can be passed to task-service for creation. No LLM; replaced in Phase 4 (PRD §25).

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Define Planner interface or struct: Plan(ctx, goalID, agentID, goalText) (Graph, error). | Go Engineer | internal/planner/planner.go |
| 2 | Stub implementation: return a graph with one task (type e.g. "stub", payload empty or minimal). | Go Engineer | Same |
| 3 | Graph shape compatible with internal/tasks (Task nodes + Dependency list). | Go Engineer | Same |
| 4 | Unit test: Plan returns one task. | Go Engineer | Tests |

**Deliverables:** `internal/planner` stub; tests.

**Acceptance criteria:** Planner.Plan returns a valid single-task graph that task-service can persist.

---

### WP1.9 — Agent lifecycle (internal/agent)

**Description:** Implement `internal/agent`: AgentActor (or equivalent) that uses the kernel (Spawn, Send), holds agent identity, and can receive messages (e.g. CreateGoal, RunPlan). Integrate with kernel and tasks: when “run” is requested, call planner stub to get DAG, then create tasks via task store and trigger scheduler (or rely on scheduler loop to pick up pending tasks). Agent service will spawn this actor per agent (PRD §6, §9).

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Implement AgentActor: ID from agent_id, mailbox, handler that processes CreateGoal / RunPlan (or single “Run” command) by calling planner and task store. | Go Engineer | internal/agent/agent.go |
| 2 | On Run: create goal row if needed (goal_id, agent_id, goal_text, status); call planner.Plan; create tasks and dependencies via tasks.Store; leave tasks in pending/created so scheduler finds ready ones. | Go Engineer | Same |
| 3 | Use internal/kernel for Send if needed; internal/planner, internal/tasks.Store. | Go Engineer | Same |
| 4 | Unit/integration tests: agent receives Run, one task created and becomes ready after scheduler tick. | QA / Go Engineer | Tests |

**Deliverables:** `internal/agent` package; tests.

**Acceptance criteria:** AgentActor can be spawned by kernel; receiving a run request creates a goal and a single-task graph; tasks are persisted and scheduler can pick them up.

---

### WP1.10 — gRPC services and API surface

**Description:** Wire kernel and task APIs to gRPC. Implement KernelService (SpawnActor, SendMessage, QueryState, SubscribeStream, PublishEvent) and TaskService (CreateTask, ScheduleTask, CompleteTask, FailTask, GetTask, GetGraph) per proto/kernel/kernel.proto and proto/tasks/task.proto. QueryState can return agent/task state from Postgres or in-memory registry. SubscribeStream can forward Redis stream to client stream.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Generate Go code from proto (buf generate); ensure go_package paths are correct. | Go Engineer | proto/*/ *.pb.go |
| 2 | Implement KernelService server: SpawnActor, SendMessage (delegate to internal/kernel); QueryState (read from DB or kernel registry); PublishEvent (internal/messaging.Publish). SubscribeStream: optional for Phase 1 (return stream of events from Redis). | Go Engineer | cmd/agent-service or shared kernel gRPC server |
| 3 | Implement TaskService server: CreateTask, ScheduleTask, CompleteTask, FailTask, GetTask, GetGraph using internal/tasks.Store and Transition. | Go Engineer | cmd/task-service |
| 4 | Use pkg/grpc for server setup and interceptors. | Go Engineer | Same |
| 5 | Integration tests: call CreateTask, GetTask, Transition via gRPC client. | QA / Go Engineer | Tests |

**Deliverables:** gRPC servers for Kernel and Task services; integration tests.

**Acceptance criteria:** Clients can create tasks and perform state transitions via gRPC; kernel Spawn/Send callable via gRPC.

---

### WP1.11 — cmd/entrypoints

**Description:** Implement or wire cmd/api-gateway, cmd/agent-service, cmd/scheduler-service, cmd/task-service, cmd/execution-worker (stub). Each main.go: load config, connect to DB and Redis, start gRPC (and optionally HTTP) server or loop. execution-worker: consume from astra:tasks:shard:0, call task-service CompleteTask (stub: no real tool run). Auth: placeholder middleware that accepts all requests (PRD §25).

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | cmd/agent-service: start kernel, register AgentActor(s), start gRPC server exposing KernelService (and agent lifecycle RPCs if any). | Go Engineer | cmd/agent-service/main.go |
| 2 | cmd/task-service: connect DB and Redis, start TaskService gRPC server. | Go Engineer | cmd/task-service/main.go |
| 3 | cmd/scheduler-service: connect DB and Redis, create Scheduler, run scheduler.Run(ctx) in a loop. | Go Engineer | cmd/scheduler-service/main.go |
| 4 | cmd/execution-worker: Consume astra:tasks:shard:0, for each message call task-service CompleteTask with stub result (e.g. empty result). Optionally Transition to scheduled/running before complete. | Go Engineer | cmd/execution-worker/main.go |
| 5 | cmd/api-gateway: REST health check + gRPC proxy to agent-service and task-service (or direct gRPC). Placeholder auth middleware. | Go Engineer | cmd/api-gateway/main.go |
| 6 | All binaries build and start without panic; deploy script can start them (scripts/deploy.sh). | DevOps / Go Engineer | deploy.sh updated if needed |

**Deliverables:** Five runnable binaries; deploy script runs them.

**Acceptance criteria:** `go build ./cmd/...` succeeds; running api-gateway, agent-service, task-service, scheduler-service, execution-worker allows end-to-end flow.

---

### WP1.12 — End-to-end and integration tests

**Description:** Integration test: spawn agent (via API or gRPC) → create goal → planner stub produces DAG → task-service creates task(s) → scheduler dispatches to stream → execution-worker consumes and completes task → event in Postgres; query state returns correct status. Use testcontainers for Postgres and Redis if not already.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Integration test: full flow with real DB and Redis (or testcontainers). | QA / Go Engineer | tests/integration/ or similar |
| 2 | Verify events table contains TaskQueued, TaskCompleted (or equivalent) and task status is completed. | QA | Test assertions |
| 3 | Document how to run the E2E test (README or docs). | Go Engineer | docs/ or tests/README |

**Deliverables:** Integration test suite; docs.

**Acceptance criteria:** Single E2E test passes: spawn → goal → plan → schedule → worker complete → events persisted.

---

## 4. Delegation hints

| Work package | Primary owner | Hand-off |
|--------------|---------------|----------|
| WP1.1 pkg/* | Go Engineer | Hand off to all other packages; DevOps ensures CI |
| WP1.2 events | Go Engineer | Hand off to tasks (WP1.6) for event sourcing |
| WP1.3 messaging | Go Engineer | Hand off to scheduler and execution-worker |
| WP1.4 actors | Go Engineer | Hand off to kernel (WP1.5) |
| WP1.5 kernel | Go Engineer | Hand off to agent (WP1.9) and gRPC (WP1.10) |
| WP1.6 tasks | Go Engineer | Hand off to scheduler, agent, task-service |
| WP1.7 scheduler | Go Engineer | Hand off to scheduler-service cmd |
| WP1.8 planner | Go Engineer | Hand off to agent |
| WP1.9 agent | Go Engineer | Hand off to agent-service cmd |
| WP1.10 gRPC | Go Engineer | Hand off to cmd and QA for integration |
| WP1.11 cmd | Go Engineer + DevOps | QA runs E2E |
| WP1.12 E2E | QA + Go Engineer | Sign-off for phase |

---

## 5. Ordering within Phase 1

1. **First (parallel possible):** WP1.1 (pkg/*). Must finish before any internal package.
2. **Next (parallel):** WP1.2 (events), WP1.3 (messaging), WP1.4 (actors).
3. **Then:** WP1.5 (kernel) after WP1.4; WP1.6 (tasks) after WP1.2.
4. **Then:** WP1.7 (scheduler) after WP1.6 and WP1.3; WP1.8 (planner) can start after WP1.6.
5. **Then:** WP1.9 (agent) after WP1.5, WP1.6, WP1.8.
6. **Then:** WP1.10 (gRPC) after WP1.5, WP1.6; WP1.11 (cmd) after WP1.7, WP1.9, WP1.10.
7. **Last:** WP1.12 (E2E) after all cmd and services are runnable.

---

## 6. Risks / open decisions

- **Event payload shape:** events table payload is JSONB; agree on schema for TaskCompleted, TaskFailed (e.g. task_id, status, result/error) so consumers and audit are consistent.
- **Single shard:** Phase 1 uses one shard (astra:tasks:shard:0). Multi-shard and shard assignment are Phase 5 or later; document assumption.
- **Auth placeholder:** api-gateway “accept all” is explicit for Phase 1; full JWT/mTLS is Phase 4. Ensure no production credentials in code.
- **Worker stub:** execution-worker only marks tasks complete; no sandbox or tool run. Phase 2 replaces with real execution.

---

## Sign-off (Phase 1 complete)

- [ ] Spawn agent → create goal → planner stubs DAG → scheduler dispatches → worker stubs complete task → events in Postgres.
- [ ] Query state (via gRPC or API) returns correct task and agent data.
- [ ] All unit and integration tests pass; E2E test passes.
- [ ] CI green; no new linter or vet issues.
