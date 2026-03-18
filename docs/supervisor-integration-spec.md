# Supervisor integration — spec

## Decision

- **Integration:** Handler-wrapper in agent-service. The kernel does not know about the supervisor. When an agent’s message handler panics or returns an error, agent-service wraps the handler so that it recovers panics and invokes the supervisor; the kernel is not modified.
- **Restart semantics:** On handler panic or error, call `Supervisor.HandleFailure(agentID)`. If the policy returns **Terminate**, call `kernel.Stop(agentID)` so the actor is removed; the agent row stays in the DB (status can be updated to 'error' or left as-is). If the policy returns **RestartBackoff**, optionally after a backoff respawn the agent via `NewFromExisting(agentID, name, ...)` and `kernel.Spawn` (same ID, same DB row). For a minimal P2 implementation, **Terminate** only is sufficient: on circuit breaker (too many restarts in window) we stop the actor; no automatic respawn.

## Agent-service wiring

- Create one `Supervisor` at startup (e.g. `RestartBackoff`, maxRestarts 3, window 1m). Pass it to the agent factory (or a wrapper that the factory uses).
- When creating an agent (SpawnActor and restore), register the agent’s actor with the supervisor (`Watch(actor)`). The agent’s handler is wrapped: `defer func() { if r := recover(); r != nil { supervisor.HandleFailure(agentID); if policy == Terminate { kernel.Stop(agentID) } } }()`, and on handler error return do the same.
- Ensure the actor’s mailbox loop (or the kernel’s Send path) does not swallow panics so the wrapper can recover. If the handler is invoked from a goroutine in BaseActor, the recover must be in that goroutine (in the wrapper around the handler).

## Same ID and DB row

- On restart (if implemented), use `NewFromExisting(existingAgentID, name, ...)` so the agent keeps the same ID and DB row; no new row is inserted.
