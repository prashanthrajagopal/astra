# API Contract Reference

Quick reference for the Astra gRPC/protobuf API contracts. Full definitions in `/proto/`.

## Kernel API (kernel.proto)

The kernel exposes these RPC endpoints:

| RPC | Request | Response | Purpose |
|---|---|---|---|
| `SpawnActor` | `SpawnActorRequest` | `SpawnActorResponse` | Create and start a new actor |
| `SendMessage` | `SendMessageRequest` | `SendMessageResponse` | Send message to actor mailbox |
| `CreateTask` | `CreateTaskRequest` | `CreateTaskResponse` | Create a new task node in a graph |
| `ScheduleTask` | `ScheduleTaskRequest` | `ScheduleTaskResponse` | Mark task as scheduled, push to worker queue |
| `CompleteTask` | `CompleteTaskRequest` | `CompleteTaskResponse` | Mark task completed with result |
| `FailTask` | `FailTaskRequest` | `FailTaskResponse` | Mark task failed with error |
| `QueryState` | `QueryStateRequest` | `QueryStateResponse` | Query entity state (agents, tasks, workers) |
| `SubscribeStream` | `SubscribeStreamRequest` | stream `Event` | Subscribe to a Redis stream |
| `PublishEvent` | `PublishEventRequest` | `PublishEventResponse` | Publish event to a stream |

## Actor Message Format (JSON / protobuf)

```json
{
  "id": "uuid",
  "type": "TaskStarted",
  "source": "worker-123",
  "target": "agent-abc",
  "payload": {"task_id": "...", "meta": {}},
  "timestamp": "2026-03-09T..."
}
```

## Core Message Types

### Goal Lifecycle
- `GoalCreated`, `PlanRequested`, `PlanGenerated`

### Task Lifecycle
- `TaskCreated`, `TaskScheduled`, `TaskStarted`, `TaskCompleted`, `TaskFailed`

### Memory
- `MemoryWrite`, `MemoryRead`

### Tool Execution
- `ToolExecutionRequested`, `ToolExecutionCompleted`

### Evaluation
- `EvaluationRequested`, `EvaluationCompleted`

## Redis Streams (wire protocol)

| Stream | Fields | Purpose |
|---|---|---|
| `astra:events` | event_id, type, actor_id, payload, timestamp | Global event stream |
| `astra:tasks:shard:<n>` | task_id, graph_id, agent_id, task_type, payload, priority, created_at | Shard-specific task queue |
| `astra:agent:events` | agent_id, event_type, payload, timestamp | Agent lifecycle events |
| `astra:worker:events` | worker_id, event_type, task_id, metadata, timestamp | Worker events |
| `astra:evaluation` | task_id, evaluator_id, result, metadata, timestamp | Evaluation results |

All streams use consumer groups. Messages are acknowledged after processing.

## REST API (api-gateway)

The api-gateway exposes REST endpoints that proxy to gRPC services:

| Method | Path | Maps to |
|---|---|---|
| `POST` | `/v1/agents` | agent-service → SpawnActor |
| `GET` | `/v1/agents/:id` | agent-service → QueryState |
| `POST` | `/v1/goals` | goal-service → GoalCreated |
| `POST` | `/v1/tasks` | task-service → CreateTask |
| `GET` | `/v1/tasks/:id` | task-service → QueryState |
| `GET` | `/v1/graphs/:id` | task-service → QueryState |
| `GET` | `/health` | Health check (no auth) |

All endpoints except `/health` require JWT authentication.

## Performance Requirement

All read API calls must respond in **≤ 10ms** by reading from Redis/Memcached. Write paths persist to Postgres and emit events asynchronously.
