# Task stream shard ownership — spec

## Hash key and function

- **Shard key:** Use **agent_id** (UUID of the agent that owns the goal/task). This keeps all tasks for a given agent on the same shard, which helps with ordering and locality when scaling workers.
- **Hash function:** `hash := crc32 or fnv1a(agent_id bytes); shardIndex := hash % shardCount`. Use a stable, fast hash (e.g. `hash/fnv1a` or `encoding/binary` on UUID) so the same agent always maps to the same shard.
- **Stream name:** `astra:tasks:shard:{shardIndex}` where `shardIndex` is in `[0, shardCount)`.

## Configuration

- **TASK_SHARD_COUNT:** env var, default 1. When 1, behavior is unchanged (single stream `astra:tasks:shard:0`). When > 1, scheduler and worker-manager compute shard from task’s agent_id and publish/requeue to the corresponding stream.

## Topology

- **Scheduler:** For each task to be enqueued, compute `shard = hash(agent_id) % TASK_SHARD_COUNT`, publish to `astra:tasks:shard:{shard}`.
- **Worker-manager (requeue):** When requeuing an orphaned task, recompute shard from that task’s agent_id and publish to the same shard stream.
- **Execution-worker:** Either (a) **multi-shard:** one consumer group per stream, run one `Consume` loop per shard (goroutine per shard), or (b) **single-shard:** each worker instance is assigned one shard (e.g. via env `TASK_SHARD_INDEX`), consumes only that shard. Recommended for simplicity: **multi-shard** — each execution-worker runs one Consume per shard so a single instance can process tasks from all shards.
- **Scaling:** Increase `TASK_SHARD_COUNT` to add more streams; add more execution-worker replicas; each replica consumes all shards (no shard-to-worker affinity required). Scheduler replicas can run in parallel; they all read from DB and publish to the same set of streams (no coordination needed beyond DB).

## Runbook

- Document in runbook: how to set `TASK_SHARD_COUNT`, how to scale workers, that shard is derived from agent_id and that changing shard count does not re-shard existing pending messages (only new publishes use the new count).
