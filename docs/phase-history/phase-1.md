# Phase 1 — Kernel MVP (completed)

**Phase:** 1
**Status:** completed
**Scope:** Actor runtime, kernel, events, messaging, tasks, scheduler, agent, planner stub, gRPC services, REST gateway, execution worker, tests.

---

## Goals

- Deliver a working kernel MVP: spawn agent → create goal → planner stubs DAG → scheduler dispatches → worker completes tasks → events in Postgres → query state returns correct data.

---

## What was done

### Internal packages (real implementations)

- **internal/actors** — BaseActor with mailbox (1024 buffer), supervisor with restart policies, circuit breaker. Unit tests.
- **internal/kernel** — Kernel with actor registry (sync.RWMutex), Spawn, Send, Stop, ActorCount. Metrics integration. Unit tests.
- **internal/events** — Append-only event store in Postgres (Insert, Replay by actor_id). Integration test.
- **internal/messaging** — Redis Streams Bus: Publish, Consume with consumer groups, XAck, PublishReturnID.
- **internal/tasks** — Task/Graph/Dependency types, Store with:
  - CreateTask, CreateGraph (tx: insert tasks + deps, auto-transition root tasks to pending)
  - Transition (UPDATE tasks + INSERT events in tx)
  - FindReadyTasks (FOR UPDATE SKIP LOCKED)
  - GetTask, GetGraph, CompleteTask, FailTask (with retry logic)
  - Unit tests for types and status constants.
- **internal/scheduler** — 100ms tick, finds ready tasks, transitions to queued, publishes to Redis `astra:tasks:shard:0`.
- **internal/agent** — AgentActor: on CreateGoal, inserts goal in DB, calls planner.Plan, sets AgentID on tasks, calls CreateGraph.
- **internal/planner** — Stub: returns 2-task DAG (analyze + implement). Unit test.
- **internal/kernelserver** — KernelService gRPC server: SpawnActor (creates agent via factory, inserts in agents table), SendMessage, QueryState (agents/tasks from DB), PublishEvent (to Redis stream). SubscribeStream returns Unimplemented.

### Shared packages

- **pkg/grpc** — NewServer (reflection, logging interceptor), ListenAndServe.
- **pkg/config** — Added AgentGRPCPort (env AGENT_GRPC_PORT, default 9091).
- **pkg/db, pkg/logger, pkg/metrics, pkg/models, pkg/otel** — Already real from Phase 0.

### Service entrypoints

- **cmd/task-service** — TaskService gRPC server on port 9090, connects DB, graceful shutdown.
- **cmd/agent-service** — KernelService gRPC server on port 9091, creates Kernel + Planner + tasks.Store + agent factory, graceful shutdown.
- **cmd/scheduler-service** — Scheduling loop (already real from Phase 0).
- **cmd/execution-worker** — Consumes from Redis stream, transitions tasks queued→scheduled→running→completed (10ms simulated work), uses tasks.Store.
- **cmd/api-gateway** — REST endpoints proxying to gRPC:
  - POST /agents (SpawnActor)
  - POST /agents/{id}/goals (SendMessage with CreateGoal)
  - GET /tasks/{id} (GetTask)
  - GET /graphs/{id} (GetGraph)
  - POST /tasks/{id}/complete (CompleteTask)
  - GET /health

### Tests

- **Unit tests:** internal/actors, internal/kernel, internal/tasks, internal/planner, internal/events (DB-dependent, skipped in -short), pkg/config, pkg/metrics.
- **Integration test:** tests/integration/e2e_test.go — full E2E: spawn agent → create goal → plan → schedule → worker complete → verify events. Skipped in -short mode.
- All tests pass: `go test ./... -count=1 -short`

### Build verification

- `go build ./...` — passes
- `go vet ./...` — passes

---

## Decisions

- **KernelService in separate package** (`internal/kernelserver`) to avoid import cycle between kernel and agent.
- **Agent factory** pattern: cmd/agent-service creates a factory closure that builds agents with all deps; passed to KernelGRPCServer.
- **Task-service on 9090, agent-service on 9091** — separate gRPC ports to run all services on one host.
- **Execution worker stub** — simulates 10ms work, transitions tasks to completed; replaced in Phase 2 with real tool execution.
- **SubscribeStream** — returns Unimplemented; server-side streaming deferred.

---

## Acceptance criteria (PRD §25)

- [x] Spawn agent → create goal → planner stubs DAG → scheduler dispatches → worker completes task → events in Postgres → query state returns correct data.

---

## References

- PRD: docs/PRD.md (§5 Kernel, §6 Actors, §7 Tasks, §8 Scheduler, §9 Services, §12 Events/Messaging, §25 Phase 1)
- Implementation plan: docs/implementation-plans/phase-1.md
