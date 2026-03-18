# Agent restore on startup — contract

## Owner

**Agent-service** owns the restore loop. It runs once at startup in `main()`, after the kernel and agentFactory are created and before (or immediately after) the gRPC server starts. Kernelserver does not perform restore; it only handles SpawnActor (new agents) and SendMessage/QueryState.

## Agent factory

- **SpawnActor path:** `agentFactory(name string) *agent.Agent` remains unchanged. It is used only for new agents created via the gRPC SpawnActor call; the factory creates an agent with a new UUID and the kernel spawns it.
- **Restore path:** Restore does not use the factory with an optional pre-existing ID. Instead, a separate constructor `agent.NewFromExisting(id uuid.UUID, name string, k, p, store, db)` builds an agent with the given ID and name (from the DB). The agent-service startup code queries the DB for active agents and calls `NewFromExisting` for each, then registers the actor with the kernel via `kernel.Spawn(a.actor)`. No DB insert is performed on restore (the row already exists).

## Data flow

1. On startup, agent-service runs: `SELECT id, name, COALESCE(actor_type, name) FROM agents WHERE status = 'active'`.
2. For each row, call `agent.NewFromExisting(id, name, k, p, taskStore, db)` which builds an Agent with that ID, starts its mailbox, and spawns it into the kernel (same as `New` but with existing ID).
3. SpawnActor continues to work as today (new UUID, DB insert, kernel spawn). Both paths result in an in-memory actor registered in the kernel.

## Invariants

- Only agents with `status = 'active'` are restored.
- Restore runs before the gRPC server accepts traffic so that restored agents are available for SendMessage immediately.
- If restore fails for one agent (e.g. NewFromExisting fails), log and continue with the remaining agents; do not fail the entire process.
