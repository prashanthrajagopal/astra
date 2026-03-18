# Task stream shard ownership and scaling

## Overview

Task enqueue and consumption use Redis streams. When `TASK_SHARD_COUNT` is greater than 1, tasks are spread across multiple streams (`astra:tasks:shard:0`, `astra:tasks:shard:1`, …) so that workload can be distributed.

## Shard assignment

- **Key:** Shard is derived from the task’s **agent_id** (hash of agent UUID modulo shard count). All tasks for the same agent go to the same shard.
- **Config:** `TASK_SHARD_COUNT` (default 1). Set the same value on scheduler, worker-manager, and execution-worker.
- **Streams:** `astra:tasks:shard:{0..TASK_SHARD_COUNT-1}`.

## Components

- **Scheduler:** Finds ready tasks (with agent_id), computes shard for each, publishes to `astra:tasks:shard:{shard}`.
- **Worker-manager:** When requeuing orphaned tasks, looks up each task’s agent_id, computes shard, and republishes to the same shard stream.
- **Execution-worker:** When `TASK_SHARD_COUNT` > 1, starts one consumer goroutine per shard (each consumes one stream). When 1, consumes only `astra:tasks:shard:0`.

## Scaling

- **Increase shards:** Set `TASK_SHARD_COUNT` to a higher value (e.g. 4) on all components and restart. New tasks will be distributed across the new shards. Existing pending messages remain in their current streams; no re-shard of in-flight messages.
- **Scale workers:** Add more execution-worker replicas. Each replica consumes all shards (multi-shard consumer). No per-shard affinity required.
- **Scheduler replicas:** Multiple scheduler instances can run; they all read from the same DB and publish to the same set of streams. No coordination needed beyond the DB.

## Runbook references

- See [docs/shard-ownership-spec.md](../shard-ownership-spec.md) for the full spec.
