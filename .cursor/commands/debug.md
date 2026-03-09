# Debug

Investigate a bug, error, or unexpected behavior in Astra. The user describes the problem after the command name (e.g. `tasks stuck in pending after scheduler restart`).

## Pre-requisites — Read Before Acting

1. Read `.cursor/skills/codebase-map/SKILL.md` for system orientation
2. Read `.cursor/skills/kernel-reference/SKILL.md` for kernel internals
3. Read `.cursor/skills/api-contract-reference/SKILL.md` if the issue is API-facing

## Delegation Chain

```
User → SRE Lead (triage and coordinate)
  → Debugger (read-only investigation — never writes code)
  → Project Manager (for routing the fix, once root cause is found)
    → Architect → Tech Lead → Go Engineer
```

The **Debugger** agent is read-only. It investigates and reports findings but never modifies files.

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

**Task execution flow:**
```
Goal → Planner → TaskGraph → Scheduler → Redis Stream → Worker → Execute → Complete → Unlock children
```

**Actor message flow:**
```
SendMessage → Kernel → actor location lookup → local channel or Redis Stream → Actor mailbox → Receive
```

**State persistence flow:**
```
State change → Postgres transaction (UPDATE + INSERT event) → Redis Stream publish → Consumer updates cache
```

At each stage, check:
- Is data arriving? (check Postgres records, Redis streams)
- Is data correct? (compare expected vs actual state)
- Is data fresh? (check timestamps, heartbeats)

### Step 3 — Gather Evidence

Use read-only commands:

```bash
# Check task status
psql -U astra -d astra -c "SELECT id, status, updated_at FROM tasks WHERE status='pending' ORDER BY created_at LIMIT 20"

# Check for stuck tasks
psql -U astra -d astra -c "SELECT id, status, updated_at FROM tasks WHERE status='running' AND updated_at < now() - interval '5 minutes'"

# Check Redis stream length
redis-cli XLEN astra:events
redis-cli XLEN astra:tasks:shard:0

# Check consumer group lag
redis-cli XINFO GROUPS astra:tasks:shard:0
redis-cli XPENDING astra:tasks:shard:0 worker-group

# Check worker heartbeats
redis-cli KEYS "worker:heartbeat:*"

# Check recent events
psql -U astra -d astra -c "SELECT event_type, actor_id, created_at FROM events ORDER BY id DESC LIMIT 20"

# Check Go process
docker logs astra-scheduler --tail 100
docker logs astra-worker --tail 100
```

### Step 4 — Report Root Cause

```
## Root Cause Analysis

**Problem**: {one-line description}
**Layer**: {actors | tasks | scheduler | messaging | state | workers | memory | grpc}
**Root cause**: {detailed explanation}
**Evidence**: {what was found during investigation}

## Recommended Fix

**Agent**: {which engineering agent should fix this}
**Packages to modify**: {list of packages}
**Description**: {what needs to change}
```

Then route the fix back through **Project Manager** for implementation.
