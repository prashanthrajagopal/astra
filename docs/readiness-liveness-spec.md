# Readiness vs liveness — spec

## Convention

- **Liveness:** `GET /health` returns 200 with body "ok". Indicates the process is up. Use for livenessProbe in Kubernetes; failure triggers restart.
- **Readiness:** `GET /ready` returns 200 when the service can accept traffic (dependencies OK); returns 503 when not ready (e.g. DB or Redis down). Use for readinessProbe; failure stops sending traffic to the pod.

## Services and dependency checks

| Service | HTTP | Dependencies to check in readiness |
|--------|------|------------------------------------|
| api-gateway | Yes | Postgres (optional), Redis (optional) — gateway may not use DB/Redis directly for every request but may use for auth/session |
| goal-service | Yes | Postgres, Redis (for admission/cache) |
| agent-service | gRPC only | Postgres (for restore/QueryState), Redis (if used) — no HTTP /ready; can add gRPC health or TCP probe |
| access-control | Yes | Postgres |
| task-service | gRPC only | Postgres — no HTTP /ready |
| execution-worker | No HTTP server | N/A for HTTP readiness; has Redis (Consume), Postgres (taskStore) |
| worker-manager | Yes | Postgres, Redis |
| identity | Yes | Postgres (if user store) |
| scheduler-service | Varies | Postgres, Redis |
| planner-service | Yes | Postgres (if any) |

For this implementation, add `GET /ready` to services that expose HTTP and have clear DB/Redis dependencies: **api-gateway**, **goal-service**, **access-control**, **worker-manager**, **identity**. Make the checks configurable via env `READINESS_CHECKS=db,redis` (default both where applicable). Agent-service and task-service are gRPC-only; document that liveness/readiness for them can use gRPC health check or TCP socket.

## Kubernetes / Helm

- **livenessProbe:** `GET /health` (unchanged). Restart the pod if the process is dead or stuck.
- **readinessProbe:** `GET /ready`. Stop sending traffic when the service reports not ready (e.g. DB/Redis down). Configured in [deployments/helm/astra/templates/deployment.yaml](deployments/helm/astra/templates/deployment.yaml). For gRPC-only services (e.g. agent-service when run without HTTP), use `tcpSocket` on the gRPC port or add an HTTP sidecar that exposes /ready.

## Response format

- **200:** Body "ok" or `{"ready":true}`. Dependencies OK.
- **503:** Body `{"ready":false,"reason":"<short reason>"}`. Do not send traffic.
