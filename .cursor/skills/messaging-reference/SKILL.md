# Messaging Reference

Reference for Redis Streams messaging patterns in Astra.

## Redis Streams Overview

Astra uses Redis Streams as the real-time message bus for all inter-service communication. Streams provide ordered, persistent, consumer-group-based message delivery.

## Stream Definitions

### 1. `astra:events` — Global Event Stream
All system events flow through this stream for observability and event sourcing.

| Field | Type | Description |
|---|---|---|
| `event_id` | string (UUID) | Unique event identifier |
| `type` | string | Event type (e.g., `TaskCompleted`, `GoalCreated`) |
| `actor_id` | string (UUID) | Actor that generated the event |
| `payload` | JSON string | Event-specific data |
| `timestamp` | string (RFC3339) | Event creation time |

### 2. `astra:tasks:shard:<n>` — Shard-specific Task Queues
Schedulers push ready tasks to shard-specific streams. Workers consume from their assigned shard.

| Field | Type | Description |
|---|---|---|
| `task_id` | string (UUID) | Task identifier |
| `graph_id` | string (UUID) | Parent task graph |
| `agent_id` | string (UUID) | Owning agent |
| `task_type` | string | Task type classifier |
| `payload` | JSON string | Task payload |
| `priority` | int | Execution priority (lower = higher priority) |
| `created_at` | string (RFC3339) | Creation timestamp |

Sharding key: `hash(agent_id) % shard_count` or `hash(graph_id) % shard_count`

### 3. `astra:agent:events` — Agent Lifecycle Events

| Field | Type | Description |
|---|---|---|
| `agent_id` | string (UUID) | Agent identifier |
| `event_type` | string | `AgentSpawned`, `AgentStopped`, `AgentError` |
| `payload` | JSON string | Event data |
| `timestamp` | string (RFC3339) | Event time |

### 4. `astra:worker:events` — Worker Events

| Field | Type | Description |
|---|---|---|
| `worker_id` | string (UUID) | Worker identifier |
| `event_type` | string | `Heartbeat`, `TaskClaimed`, `TaskCompleted`, `TaskFailed` |
| `task_id` | string (UUID) | Associated task (if applicable) |
| `metadata` | JSON string | Additional context |
| `timestamp` | string (RFC3339) | Event time |

### 5. `astra:evaluation` — Evaluation Results

| Field | Type | Description |
|---|---|---|
| `task_id` | string (UUID) | Evaluated task |
| `evaluator_id` | string (UUID) | Evaluator that ran |
| `result` | string | `pass`, `fail`, `partial` |
| `metadata` | JSON string | Scores, feedback |
| `timestamp` | string (RFC3339) | Evaluation time |

## Consumer Group Patterns

### Creating consumer groups
```go
client.XGroupCreateMkStream(ctx, stream, group, "0")
```

### Reading from consumer group
```go
msgs, err := client.XReadGroup(ctx, &redis.XReadGroupArgs{
    Group:    group,
    Consumer: consumerID,
    Streams:  []string{stream, ">"},
    Count:    10,
    Block:    5 * time.Second,
}).Result()
```

### Acknowledging messages
```go
client.XAck(ctx, stream, group, msg.ID)
```

### Claiming pending messages (timeout recovery)
```go
msgs, err := client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
    Stream:   stream,
    Group:    group,
    Consumer: consumerID,
    MinIdle:  30 * time.Second,
    Start:    "0",
    Count:    10,
}).Result()
```

## Redis Key Patterns

### Actor state (ephemeral working memory)
```
actor:state:<actor_id>     → Redis Hash, TTL-based
```

### Distributed locks (Redlock pattern)
```
lock:task:<task_id>        → SET NX PX <timeout>
```

### Worker heartbeat tracking
```
worker:heartbeat:<worker_id> → SET with TTL (30s)
```

## Memcached Key Patterns

### LLM response cache
```
llm:resp:{model}:{prompt_hash}  → serialized response, TTL ~24h
```

### Embedding cache
```
embed:{content_hash}            → serialized vector, TTL 7-30d
```

### Tool result cache
```
tool:cache:{tool_name}:{input_hash} → serialized result, TTL varies
```

## Error Handling

- If Redis is unavailable: circuit breaker → retry with backoff → fallback to Postgres event replay
- If consumer group read fails: exponential backoff (100ms → 200ms → 400ms → ... → 30s cap)
- Unprocessable messages: move to dead-letter stream `astra:dead_letter` after 3 failures
- Monitor `XPENDING` counts for consumer group lag alerting

## Performance Notes

- Use `XADD` with `MAXLEN ~` for stream trimming (prevent unbounded growth)
- Pipeline multiple Redis commands when publishing batch events
- Worker `XREADGROUP` with `Block: 5s` to reduce polling overhead
- Target: publish-to-consume latency < 5ms within same Redis instance
