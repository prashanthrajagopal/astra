# Kernel Reference

Reference for the Astra microkernel internals. The kernel is the minimal, stable core.

## Kernel Responsibilities (and nothing more)

1. **Actor Runtime** — Run actors as goroutines with mailbox channels, supervision trees
2. **Task Graph Engine** — Persist DAGs, resolve dependencies, manage task lifecycle
3. **Scheduler** — Shard-aware distributed scheduling, capability matching, priority queues
4. **Message Bus** — Redis Streams + local in-memory channels for actor communication
5. **State Manager** — Transactional Postgres persistence, event sourcing, snapshots

## Kernel Invariants

- Kernel must be small and stable.
- All non-kernel services run in user-space (SDK/services).
- Kernel guarantees: message delivery within SLAs, consistent task state, transactionally consistent state writes.

## Kernel API (gRPC)

```
SpawnActor(actor_spec) → ActorID
SendMessage(actorID, Message)
CreateTask(task_spec) → TaskID
ScheduleTask(taskID)
CompleteTask(taskID, result)
FailTask(taskID, error)
QueryState(entity, filters)
SubscribeStream(stream_name, consumer_group)
PublishEvent(event)
```

## Actor Runtime Details

### Mailbox model
- Each actor has a buffered channel (`chan Message`, capacity 1024)
- Non-blocking sends: if mailbox full, return `ErrMailboxFull`
- Actor goroutine loops on `select` between mailbox and stop channel

### Supervision tree
```
SystemSupervisor
 └ AgentSupervisor(s)
     ├ Planner
     ├ Memory
     └ Executor
```

Policies: `RestartImmediate`, `RestartBackoff`, `Escalate`, `Terminate`
Circuit breaker limits restarts to avoid "restart storms".

### Actor persistence
- Actors managing durable state snapshot to Postgres periodically
- On restart, state restored from latest snapshot
- Snapshot stored as JSONB in actor-specific tables or as events

### Actor location
- Local: direct in-process channel send (low latency)
- Cross-node: publish to Redis Streams; kernel maps actor→node location; `SendMessage` proxies

## Task Graph Engine Details

### DAG model
- `TaskGraph` = DAG of `TaskNode`s
- Each node: id, type, agent_id, payload (JSONB), status, priority, retries, metadata

### Task lifecycle states
```
created → queued → scheduled → running → completed / failed → dead-letter
```
Every transition persists to Postgres and appends to `events` table.

### Ready task detection
```sql
SELECT t.id FROM tasks t
WHERE t.status = 'pending'
  AND NOT EXISTS (
    SELECT 1 FROM task_dependencies d
    JOIN tasks td ON td.id = d.depends_on
    WHERE d.task_id = t.id AND td.status != 'completed'
  )
FOR UPDATE SKIP LOCKED;
```

### Child unlocking
When a task completes, the scheduler checks all tasks that depend on it. If all their dependencies are now complete, they become ready.

## Scheduler Details

### Sharding
- Shard by `agent_id` or `task_graph_id` using consistent hashing
- Each scheduler instance owns a subset of shards
- Shard assignment stored in Postgres

### Scheduling loop
1. Detect ready nodes (dependencies satisfied)
2. Mark ready atomically, push to Redis stream `astra:tasks:shard:<n>`
3. Workers pull from consumer groups, claim with `SET task.status = 'scheduled'`
4. Worker executes → heartbeat → `CompleteTask` writes result, emits `TaskCompleted`
5. Scheduler unlocks children

### Worker heartbeat
- Workers send periodic heartbeats to `astra:worker:events`
- If heartbeat lost (>30s), scheduler marks task as `queued` and re-pushes
- Dead-letter queue for tasks exceeding `max_retries`

## Message Bus Details

### Redis Streams
| Stream | Purpose |
|---|---|
| `astra:events` | Global event stream |
| `astra:tasks:shard:<n>` | Per-shard task queue |
| `astra:agent:events` | Agent lifecycle events |
| `astra:worker:events` | Worker heartbeats and events |
| `astra:evaluation` | Evaluation results |

### Consumer groups
- Each stream has a consumer group per service type
- Messages acknowledged after successful processing
- Pending messages reclaimed after timeout (XCLAIM)
- Backoff on consumer errors

## State Manager Details

### Postgres as source of truth
- All durable state in Postgres
- Event sourcing: `events` table captures every state change
- Snapshots for long-lived actors to avoid full replay

### Caching layer
- Redis: `actor:state:<actor_id>` (hash, TTL for working memory)
- Distributed locks: `lock:task:<task_id>` (Redlock pattern)
- Memcached: `llm:resp:{model}:{hash}`, `embed:{hash}`, `tool:cache:{tool}:{hash}`
