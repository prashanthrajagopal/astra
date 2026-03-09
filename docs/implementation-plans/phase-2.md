# Phase 2 — Workers & Tool Runtime — Implementation Plan

**Depends on:** Phase 1 complete. Acceptance: end-to-end flow with stub worker and events in Postgres.

**PRD references:** §8 Scheduler (worker heartbeat), §9 Services (worker-manager, execution-worker, tool-runtime, browser-worker), §11 DB (workers table), §12 Message protocols (astra:worker:events), §14 Tool Runtime & Sandboxing, §25 Phase 2, §26 Build Order.

---

## 1. Phase goal

Replace the stub execution-worker with real worker registration, heartbeat, task claiming from Redis, and tool execution inside a sandbox (Docker first; WASM optional). Add worker-manager, tool-runtime service, and browser-worker so that a task can be executed in an isolated environment and results/artifacts are persisted.

---

## 2. Dependencies

- **Phase 1** complete: scheduler pushes to `astra:tasks:shard:0`; task-service exposes CompleteTask/FailTask; events and tasks tables in use.
- **DB:** `workers` table (migrations/0005_workers.sql) and indexes (0007).
- **Internal packages:** internal/workers, internal/tools; cmd/execution-worker, cmd/worker-manager, cmd/tool-runtime, cmd/browser-worker.

---

## 3. Work packages

### WP2.1 — Worker registration and heartbeat (internal/workers)

**Description:** Implement `internal/workers`: worker registration (insert/update in `workers` table: hostname, status, capabilities, last_heartbeat), heartbeat loop (publish to `astra:worker:events` and/or update DB), task claiming (read from stream, update task status to scheduled/running, optional lock). Heartbeat interval and failure detection (30s) per PRD §8, §24.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Define Worker struct and Registry interface: Register(ctx, workerID, hostname, capabilities), UpdateHeartbeat(ctx, workerID). | Go Engineer | internal/workers/worker.go |
| 2 | Implement DB-backed registry: INSERT/UPDATE workers table; last_heartbeat = now(). | Go Engineer | internal/workers/registry.go or store |
| 3 | Heartbeat publishing: publish WorkerHeartbeat to astra:worker:events (worker_id, task_id if any, timestamp) every 10s. | Go Engineer | Same |
| 4 | Optional: Redis key worker:heartbeat:<id> with TTL 30s for fast liveness check. | Go Engineer | Same |
| 5 | Unit tests: register worker, update heartbeat, query workers. | Go Engineer | Tests |

**Deliverables:** internal/workers package; tests.

**Acceptance criteria:** Worker can register; heartbeats update DB and/or Redis; worker-manager can list active workers (heartbeat within 30s).

---

### WP2.2 — Task claiming and execution loop (execution-worker)

**Description:** Execution-worker consumes from `astra:tasks:shard:0` using consumer group, claims task (Transition to scheduled then running), calls tool-runtime to execute, then CompleteTask or FailTask. Integrate with internal/tasks (Transition) and internal/tools (Runtime.Execute). Worker sends heartbeats while running.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Consume from stream via internal/messaging; parse task_id, graph_id, agent_id, task_type, payload. | Go Engineer | cmd/execution-worker |
| 2 | On claim: call task-service ScheduleTask or internal Transition to scheduled; then Transition to running; emit TaskStarted event if needed. | Go Engineer | Same |
| 3 | Call tool-runtime (gRPC or in-process) with task payload; on success call CompleteTask with result; on failure call FailTask with error. | Go Engineer | Same |
| 4 | Heartbeat during execution (e.g. every 10s) to astra:worker:events. | Go Engineer | Same |
| 5 | Timeout and FailTask if execution exceeds task timeout (configurable). | Go Engineer | Same |
| 6 | Integration test: scheduler enqueues task, worker claims and completes with mock tool. | QA / Go Engineer | Tests |

**Deliverables:** cmd/execution-worker with real execution path; tests.

**Acceptance criteria:** Worker registers, claims task from stream, runs in sandbox, completes or fails task; events and result persisted.

---

### WP2.3 — Tool runtime and sandbox (internal/tools, cmd/tool-runtime)

**Description:** Implement `internal/tools`: Runtime interface (Execute(ctx, ToolRequest) (ToolResult, error)), ToolRequest (Name, Input, Sandbox type, Timeout, MemoryLimit, CPULimit), ToolResult (Output, Artifacts, ExitCode, Duration). Docker sandbox first: launch container with resource limits, inject input, capture stdout/stderr, return output and artifact URIs. PRD §14: secrets via ephemeral volumes only; no secrets in env or logs. cmd/tool-runtime: gRPC or HTTP server that accepts Execute and delegates to internal/tools.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Define ToolRequest, ToolResult, Runtime interface in internal/tools per PRD §14. | Go Engineer | internal/tools/runtime.go |
| 2 | Implement Docker runtime: create container from configurable image, set CPU/memory limits, timeout; pass input via stdin or volume; capture stdout/stderr; optional artifact upload to MinIO/S3. | Go Engineer | internal/tools/docker.go or sandbox_docker.go |
| 3 | Enforce resource limits (CPU, memory, time) and least-privilege (no privileged mode; optional read-only root). | Go Engineer | Same |
| 4 | Document WASM/Firecracker as future; Phase 2 scope is Docker only unless otherwise decided. | Go Engineer | Comments or docs |
| 5 | cmd/tool-runtime: expose Execute RPC or HTTP; call internal/tools.Runtime. | Go Engineer | cmd/tool-runtime/main.go |
| 6 | Unit/integration tests: mock Docker or testcontainers; execute simple command, assert output and exit code. | QA / Go Engineer | Tests |

**Deliverables:** internal/tools with Docker runtime; cmd/tool-runtime; tests.

**Acceptance criteria:** ToolRuntime.Execute runs a command in a container and returns output; resource limits applied; no secrets in env (S5).

---

### WP2.4 — Worker manager service (cmd/worker-manager)

**Description:** Implement cmd/worker-manager: service that tracks worker registration and heartbeats, exposes list of active/draining workers, and optionally provides scaling hints (e.g. queue depth vs worker count). Consumes astra:worker:events or reads from workers table; marks workers offline if heartbeat missing >30s. PRD §8: worker failure detection ≤30s.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Start worker-manager: connect DB and Redis, subscribe or poll workers table and heartbeat keys. | Go Engineer | cmd/worker-manager/main.go |
| 2 | Implement ListWorkers(ctx, status filter) and optional GetWorker(ctx, id). | Go Engineer | Same |
| 3 | Background loop: mark workers offline if last_heartbeat older than 30s; optionally emit WorkerDraining/WorkerOffline events. | Go Engineer | Same |
| 4 | gRPC or REST API for dashboard/scheduler to query worker health. | Go Engineer | Same |
| 5 | Integration test: register worker, stop heartbeat, assert worker marked offline within 30s. | QA / Go Engineer | Tests |

**Deliverables:** cmd/worker-manager; tests.

**Acceptance criteria:** Worker-manager lists workers; workers not heartbeating within 30s are marked offline; scheduler can use this for re-queueing (Phase 1 or 5 may implement re-queue).

---

### WP2.5 — Scheduler: re-queue on worker failure (optional in Phase 2)

**Description:** If a worker dies (heartbeat lost), tasks that were “running” on that worker should be moved back to queued and re-pushed to the stream. This can be done in scheduler-service or worker-manager: periodic check for running tasks whose worker is offline, then Transition to queued and Publish again. PRD §20 runbook: “Move in-flight tasks: UPDATE tasks SET status='queued' WHERE worker_id=$1 AND status='running'”.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Design: who owns “re-queue in-flight tasks for dead worker”? (scheduler vs worker-manager). Recommend scheduler or dedicated reconciliation loop. | Architect / Tech Lead | Decision |
| 2 | Implement: periodic scan for tasks in status=running whose worker_id (if stored) or last_heartbeat is stale; transition to queued; push to stream. | Go Engineer | internal/scheduler or workers |
| 3 | Add worker_id to tasks table if not present (migration); execution-worker sets it when claiming. | DB Architect | Migration if needed |
| 4 | Test: worker claims task, worker dies (stop process), after 30s task is re-queued and another worker can claim. | QA | Tests |

**Deliverables:** Re-queue logic; migration if needed; tests.

**Acceptance criteria:** When worker fails, its in-flight tasks are re-queued within a bounded time (e.g. 30s + one tick).

---

### WP2.6 — Browser worker (cmd/browser-worker)

**Description:** Implement cmd/browser-worker: specialized worker that consumes tasks of type “browser” or from a dedicated stream, runs Playwright/Puppeteer (or similar) in a container, and returns artifacts (screenshots, HTML). May share task claiming pattern with execution-worker but use a different stream or task_type filter. PRD §9: browser-worker for headless browser automation.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Define browser task payload (e.g. URL, script, options). | Go Engineer | Proto or internal/types |
| 2 | Implement browser-worker: consume browser tasks (stream or filter), run headless browser in container, capture result and artifacts. | Go Engineer | cmd/browser-worker/main.go |
| 3 | Use same worker registration/heartbeat pattern as execution-worker. | Go Engineer | Same |
| 4 | Integration test: submit browser task, worker returns screenshot or HTML. | QA / Go Engineer | Tests |

**Deliverables:** cmd/browser-worker; tests.

**Acceptance criteria:** Browser-worker can run a simple browser task and return artifacts.

---

## 4. Delegation hints

| Work package | Primary owner | Hand-off |
|--------------|---------------|----------|
| WP2.1 workers | Go Engineer | Hand off to execution-worker and worker-manager |
| WP2.2 execution-worker | Go Engineer | Depends on WP2.1, WP2.3; QA integration test |
| WP2.3 tools + tool-runtime | Go Engineer | Hand off to execution-worker; DevOps for Docker image/config |
| WP2.4 worker-manager | Go Engineer | QA for heartbeat/failure test |
| WP2.5 re-queue | Go Engineer | May need DB Architect for worker_id on tasks |
| WP2.6 browser-worker | Go Engineer | QA for browser E2E |

---

## 5. Ordering within Phase 2

1. **First:** WP2.1 (internal/workers) — needed for registration and heartbeat.
2. **Parallel:** WP2.3 (internal/tools + tool-runtime) and WP2.4 (worker-manager) can proceed after WP2.1.
3. **Then:** WP2.2 (execution-worker) after WP2.1 and WP2.3.
4. **Then:** WP2.5 (re-queue) after WP2.2; may require migration (worker_id on tasks) — DB Architect.
5. **Then or parallel:** WP2.6 (browser-worker) after WP2.1 and task format for browser.

---

## 6. Risks / open decisions

- **Docker image:** Default image for “run command” tasks must be agreed (e.g. alpine, or custom astra-worker image). Document in deploy or values.
- **WASM vs Docker priority:** PRD §14 lists WASM first; Phase 2 says “Docker first, WASM later.” Confirm with product: Phase 2 = Docker only.
- **worker_id on tasks:** If re-queue requires knowing which worker had the task, add tasks.worker_id column and migration; update on claim.
- **Secrets in sandbox:** S5 requires secrets via ephemeral volumes; no env. Implement in WP2.3 if any task needs secrets.

---

## Sign-off (Phase 2 complete)

- [ ] Worker registers and sends heartbeats; worker-manager lists active workers and marks offline after 30s.
- [ ] Execution-worker claims task from Redis, executes in Docker sandbox, completes/fails and persists result.
- [ ] Tool-runtime enforces resource limits and returns artifacts; no secrets in env.
- [ ] Optional: in-flight tasks of dead worker are re-queued.
- [ ] Browser-worker runs browser tasks and returns artifacts.
- [ ] All tests pass; CI green.
