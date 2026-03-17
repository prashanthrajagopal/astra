# Codebase Map

Quick reference for the Astra Autonomous Agent OS repository structure. Read this first to orient yourself.

## Repository layout

```
astra/
├── cmd/                        # Service entrypoints (one folder per service)
│   ├── api-gateway/            # REST/gRPC gateway, auth, versioning
│   │   └── dashboard/          # Super-admin UI: index.html, static/style.css, app.js
│   ├── identity/               # User/service auth, tokens, audit log
│   ├── access-control/         # RBAC, OPA policy enforcement
│   ├── agent-service/          # Agent lifecycle, actor supervisor integration
│   ├── goal-service/           # Goal ingestion, validation, routing
│   ├── planner-service/        # Goals → TaskGraphs
│   ├── scheduler-service/      # Distributed scheduler, shard manager
│   ├── task-service/           # Task CRUD, dependency engine API
│   ├── llm-router/             # Model routing, caching, rate limiting
│   ├── prompt-manager/         # Prompt templates, versions, A/B
│   ├── evaluation-service/     # Result validators, auto-evaluators
│   ├── worker-manager/         # Worker registration, health, scaling hints
│   ├── execution-worker/       # General task execution runtime
│   ├── browser-worker/         # Headless browser automation
│   ├── tool-runtime/           # Tool sandbox controller (WASM/Docker/Firecracker)
│   └── memory-service/         # Episodic/semantic memory, embedding pipelines
├── internal/                   # Private implementation packages
│   ├── actors/                 # Kernel actor runtime (BaseActor, mailbox, supervision)
│   ├── agent/                  # Agent-level orchestration, AgentActor
│   ├── kernel/                 # Kernel manager (Spawn, Send, message routing)
│   ├── kernelserver/           # Kernel gRPC server (SpawnActor, SendMessage, QueryState), wraps kernel + DB
│   ├── planner/                # Planner orchestration, plan validators
│   ├── scheduler/              # Scheduling loop, shard management, ready-task detection
│   ├── tasks/                  # Task model, state machine, transitions, persistence
│   ├── memory/                 # Memory APIs, embedding pipeline, pgvector search
│   ├── workers/                # Worker orchestration, heartbeats, health
│   ├── tools/                  # Tool runtime control, sandbox lifecycle
│   ├── evaluation/             # Evaluators, test harness integration
│   ├── events/                 # Event store, event replay, event sourcing
│   └── messaging/              # Redis Streams clients, consumer groups, backoff, ack
├── pkg/                        # Shared packages (stable, versioned, documented)
│   ├── db/                     # DB connection, migration runner, helpers
│   ├── config/                 # Config loader (env, Vault)
│   ├── logger/                 # Structured logging (slog/zerolog)
│   ├── metrics/                # Prometheus metrics registration
│   ├── grpc/                   # gRPC server/client helpers, interceptors
│   └── models/                 # Shared domain types
├── proto/                      # Protobuf/gRPC definitions
│   ├── kernel.proto            # Kernel API (SpawnActor, SendMessage, etc.)
│   └── task.proto              # Task API (CreateTask, ScheduleTask, etc.)
├── migrations/                 # SQL migration files (ordered)
│   ├── 0001_initial_schema.sql
│   ├── 0002_task_dependencies.sql
│   ├── 0003_memories_pgvector.sql
│   ├── 0004_artifacts.sql
│   ├── 0005_workers.sql
│   ├── 0006_indexes.sql
│   ├── 0007_events.sql
│   └── 0008_constraints.sql
├── deployments/                # Helm charts, k8s manifests
│   └── helm/astra/
│       ├── Chart.yaml
│       ├── values.yaml
│       └── templates/
├── web/                        # Frontend (future)
├── scripts/                    # deploy.sh (local), gcp-deploy.sh (GCP/GKE/GCS), seed-agents.sh
├── docs/                       # Architecture docs, runbooks
├── tests/                      # Integration/e2e test fixtures
├── source/                     # PRD and reference scaffolds
│   └── prd source.md           # Complete PRD & Engineering Spec
└── .cursor/
    ├── agents/                 # Agent definitions
    ├── rules/                  # Cursor rules
    ├── skills/                 # Agent skills (this directory)
    └── commands/               # Slash commands
```

## Kernel Components (internal/)

| Package | Responsibility | Key Types |
|---|---|---|
| `actors` | Actor runtime primitives | `Actor` interface, `BaseActor`, `Message`, `Supervisor` |
| `agent` | Agent lifecycle orchestration | `Agent`, `AgentActor` |
| `kernel` | Kernel manager (in-process) | `Kernel`, Spawn, Send, Stop |
| `kernelserver` | Kernel gRPC server (SpawnActor, SendMessage, QueryState) | `KernelGRPCServer`, reads gRPC metadata (x-org-id, x-is-super-admin) for agent listing |
| `tasks` | Task state machine and DAG | `Task`, `Graph`, `Status` |
| `scheduler` | Distributed scheduling | `Scheduler`, shard ownership, ready-set detection |
| `messaging` | Redis Streams abstraction | `Bus`, consumer groups, publish/subscribe |
| `events` | Event sourcing | `Event`, event store, replay |
| `memory` | Agent memory system | `Memory`, embedding search, pgvector |
| `workers` | Worker pool management | heartbeats, task assignment |
| `tools` | Tool sandbox control | WASM/Docker/Firecracker lifecycle |
| `evaluation` | Result validation | `Evaluator`, test harness |
| `planner` | Plan generation | `Planner`, goal → DAG conversion |

## Infrastructure Dependencies

| Service | Purpose | Port |
|---|---|---|
| PostgreSQL | Source of truth, pgvector | 5432 |
| Redis | Streams, ephemeral state, locks | 6379 |
| Memcached | LLM cache, embedding cache | 11211 |
| MinIO (local/docker-compose) | Optional artifact storage | 9000 |
| GCS (GCP) | Workspace/object bucket via `gcp-deploy.sh` (`gs://$PROJECT-astra-workspace`); MinIO not used on GCP deploy path | — |
