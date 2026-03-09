# Observability

Astra uses Prometheus metrics, structured logs, and OpenTelemetry-compatible tracing hooks.

## Metrics

Primary `astra_*` metrics:

- `astra_task_latency_seconds`
- `astra_task_success_total`
- `astra_task_failure_total`
- `astra_scheduler_ready_queue_depth`
- `astra_actor_count`
- `astra_llm_token_usage_total`
- `astra_llm_cost_dollars`
- `astra_llm_cost_dollars_total`
- `astra_worker_heartbeat_total`
- `astra_events_processed_total`

## Tracing

- Services should propagate context across gRPC/HTTP boundaries.
- Logs should include correlation identifiers (request_id / actor_id / task_id where available).
- OTLP endpoint is configured by `OTEL_EXPORTER_OTLP_ENDPOINT`.

## Dashboards and Alerts

- Dashboards: `deployments/grafana/dashboards/`
- Alert rules: `deployments/prometheus/rules/astra-alerts.yaml`
- Runbooks: `docs/runbooks/`

## Operational Flow Coverage

Task lifecycle observability path:

1. Goal/task creation events in `events` table
2. Scheduler queue depth + dispatch metrics
3. Worker heartbeat + completion/failure metrics
4. LLM usage/cost telemetry and async audit persistence
