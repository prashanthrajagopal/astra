# Gateway circuit breaker — spec

## Parameters

- **Failure threshold:** 5 failures in the window open the circuit.
- **Window:** 30 seconds (sliding or fixed; failures outside the window do not count).
- **Half-open cooldown:** 10 seconds. After this period the circuit allows one request through; if it succeeds the circuit closes, if it fails the circuit reopens.
- **Per-target state:** One circuit breaker per downstream (goal-service, agent-service, access-control). Optionally one for task-service if the gateway calls it.

## Gateway routes that use the circuit breaker

| Downstream | Used by (gateway routes) |
|------------|---------------------------|
| goal-service (HTTP) | POST /agents/{id}/goals (handleAgentGoalsProxy), GET /superadmin/api/dashboard/goals/{id}, POST .../goals/{id}/cancel, chat/sessions append message |
| agent-service (gRPC) | SpawnActor (POST /agents), SendMessage (CreateGoal legacy path unused now), QueryState (GET agents list, dashboard snapshot) |
| access-control (HTTP) | POST /check (auth), GET/POST approvals (handleGetApprovalProxy, handleApprovalActionProxy) |
| task-service (gRPC) | Optional: CompleteTask, GetTask, etc. if gateway proxies task operations |

## Behavior when circuit is open

- Return **503 Service Unavailable** with body `{"error":"service temporarily unavailable"}`.
- Optional: set header `Retry-After: 60` (seconds) to suggest client backoff.
- Log at warn level that the circuit is open for the given target.

## Configuration

- Env: `CIRCUIT_BREAKER_THRESHOLD=5`, `CIRCUIT_BREAKER_WINDOW_SEC=30`, `CIRCUIT_BREAKER_COOLDOWN_SEC=10`. Defaults as above if unset.
