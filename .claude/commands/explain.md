# Explain

Explain how a part of Astra works. Describe what you want to understand after invoking (e.g., `how the scheduler detects ready tasks and dispatches them to workers`).

This is a **read-only investigation**. Do not modify any files.

## Step 1 — Identify the Scope

Map the question to the relevant part of the system:

| Topic | Key Files |
|-------|-----------|
| Actor runtime, mailboxes, supervision | `internal/actors/` |
| Agent lifecycle, AgentActor | `internal/agent/` |
| Task graphs, DAGs, state machine | `internal/tasks/` |
| Scheduling, sharding, ready-task detection | `internal/scheduler/` |
| Redis Streams, consumer groups | `internal/messaging/` |
| Event sourcing, event replay | `internal/events/` |
| Agent memory, embeddings, pgvector | `internal/memory/` |
| Tool sandbox, WASM/Docker/Firecracker | `internal/tools/` |
| Worker pool, heartbeats | `internal/workers/` |
| Evaluation, validators | `internal/evaluation/` |
| Planning, goal → DAG conversion | `internal/planner/` |
| gRPC API, protobuf contracts | `proto/`, `pkg/grpc/` |
| Database schema, migrations | `migrations/`, `.cursor/skills/db-schema-reference/` |
| Deployment, k8s, Helm | `deployments/` |
| Service entrypoints | `cmd/*/main.go` |

## Step 2 — Trace the Flow

Read the relevant source files and trace execution. Follow imports, method calls, and data transformations.

## Step 3 — Explain

Provide:

1. **Overview** — One paragraph summary
2. **Data Flow** — Mermaid diagram showing the flow
3. **Step-by-Step** — Numbered steps with file references
4. **Key Files** — Table of the most important files
5. **Configuration** — What can be changed via config or env vars
