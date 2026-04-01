# Debug

Investigate a bug, error, or unexpected behavior in Astra. Describe the problem after invoking (e.g., `tasks stuck in pending after scheduler restart`).

## Pre-requisites — Read Before Acting

1. Read `.cursor/skills/codebase-map/SKILL.md` for system orientation
2. Read `.cursor/skills/kernel-reference/SKILL.md` for kernel internals
3. Read `.cursor/skills/api-contract-reference/SKILL.md` if the issue is API-facing

## Investigation Framework

### Step 1 — Classify the Problem

| Layer | Symptoms | Key Packages |
|-------|----------|-------------|
| Actor Runtime | Actors not receiving messages, goroutine leaks, supervision failures | `internal/actors/` |
| Task Graph | Tasks stuck in pending, dependency resolution failures, DAG corruption | `internal/tasks/` |
| Scheduler | Ready tasks not dispatched, shard imbalance, heartbeat timeouts | `internal/scheduler/` |
| Message Bus | Redis stream lag, consumer group stalls, message loss | `internal/messaging/` |
| State Manager | Postgres connection errors, deadlocks, event gaps | `internal/events/`, `pkg/db/` |
| Workers | Worker heartbeat lost, task requeue storms, sandbox failures | `internal/workers/`, `internal/tools/` |
| Memory Service | Embedding search failures, pgvector index issues | `internal/memory/` |
| gRPC Layer | Connection refused, deadline exceeded, auth failures | `pkg/grpc/`, `cmd/*/` |

### Step 2 — Trace the Data Flow

**Task execution:** Goal → Planner → TaskGraph → Scheduler → Redis Stream → Worker → Execute → Complete → Unlock children

**Actor messages:** SendMessage → Kernel → actor location lookup → local channel or Redis Stream → Actor mailbox → Receive

**State persistence:** State change → Postgres tx (UPDATE + INSERT event) → Redis Stream publish → Consumer updates cache

### Step 3 — Gather Evidence

Use read-only commands to inspect state:
- Postgres: task status, stuck tasks, recent events
- Redis: stream lengths, consumer group lag, pending messages, worker heartbeats
- Logs: service container/process logs

### Step 4 — Report Root Cause

```
## Root Cause Analysis

**Problem**: {one-line description}
**Layer**: {actors | tasks | scheduler | messaging | state | workers | memory | grpc}
**Root cause**: {detailed explanation}
**Evidence**: {what was found}

## Recommended Fix

**Packages to modify**: {list}
**Description**: {what needs to change}
```
