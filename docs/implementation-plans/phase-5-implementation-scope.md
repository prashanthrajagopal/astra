# Phase 5 — Implementation Scope (Concrete Checklist)

**Status:** In-repo deliverable scope — Phase 4 complete.  
**Source plan:** `docs/implementation-plans/phase-5.md`  
**Mandatory:** WP5.1–WP5.7. WP5.8 (sharding) optional.

---

## WP5.1 — Load Tests

### Files to Create/Update

| Action | Path |
|--------|------|
| CREATE | `tests/load/README.md` — Test plan: N agents × M goals × K tasks; sustained vs burst; targets (10k agents, 1M tasks/day; p99 read ≤10ms, median scheduling ≤50ms, P95 ≤500ms) |
| CREATE | `tests/load/k6-config.js` (or `tests/load/main.go` if Go-based) — Load harness driving api-gateway (POST /agents, POST /agents/{id}/goals, GET /health) with configurable concurrency |
| CREATE | `tests/load/scenarios.json` — Scenario definitions (agents per run, goals per agent, VUs, duration) |
| CREATE | `tests/load/results/` — Placeholder dir for `results report.md` template |
| UPDATE | `docs/implementation-plans/phase-5.md` — Add reference to `tests/load/` |

### Acceptance

- [ ] Load test runs (`k6 run tests/load/k6-config.js` or `go run tests/load/main.go`) without error when api-gateway is up
- [ ] Results (latency percentiles, throughput, errors) documented in `tests/load/results/` or inline
- [ ] Either SLOs met or remediation plan in `tests/load/README.md`

---

## WP5.2 — Grafana Dashboards

### Files to Create/Update

| Action | Path |
|--------|------|
| CREATE | `deployments/grafana/dashboards/cluster-overview.json` — Cluster Overview: capacity, active agents, task throughput, error rate, queue depth (Prometheus queries) |
| CREATE | `deployments/grafana/dashboards/agent-health.json` — Agent Health: per-agent throughput, avg latency, failure rate |
| CREATE | `deployments/grafana/dashboards/cost.json` — Cost: LLM token usage and cost per agent, per model, per day (`astra_llm_token_usage_total`, `astra_llm_cost_dollars`, or llm_usage aggregation) |
| CREATE | `deployments/grafana/provisioning/dashboards.yml` — Provisioning config (optional; or document manual import) |
| CREATE | `deployments/grafana/README.md` — How to import dashboards; datasource (Prometheus) requirements |

### Acceptance

- [ ] JSON files are valid Grafana dashboard format
- [ ] Dashboards reference Prometheus metrics (`astra_*`) or documented alternative
- [ ] README describes import steps

---

## WP5.3 — Prometheus Alert Rules

### Files to Create/Update

| Action | Path |
|--------|------|
| CREATE | `deployments/prometheus/rules/astra-alerts.yaml` — Rules: task failure rate >5% over 5min, queue depth >10k, worker availability <50%, LLM cost spike >2× daily avg |
| CREATE | `deployments/prometheus/rules/README.md` — Severity and runbook links per alert |
| UPDATE | `deployments/helm/astra/values.yaml` or create `deployments/prometheus/values.yaml` — Mount rules if Prometheus is subchart; else document for operator |

### Acceptance

- [ ] Alert rules YAML is valid Prometheus format
- [ ] Each alert has `runbook_url` or doc reference
- [ ] Rules fire under defined conditions (verified in staging or documented)

---

## WP5.4 — Runbooks

### Files to Create/Update

| Action | Path |
|--------|------|
| CREATE | `docs/runbooks/worker-lost.md` — Worker Lost: last heartbeat, check stream, move in-flight to queued, restart |
| CREATE | `docs/runbooks/high-error-rate.md` — High Error Rate: sample failed tasks, traces, pause intake, rollback |
| CREATE | `docs/runbooks/db-redis.md` — Postgres/Redis connectivity and failover |
| CREATE | `docs/runbooks/llm-cost-spike.md` — LLM cost spike: disable premium routing, enforce lower tiers |
| CREATE | `docs/runbooks/README.md` — Index of runbooks with severity and alert mapping |

### Acceptance

- [ ] Each runbook has: steps to detect, triage, contain, remediate
- [ ] References Grafana dashboards and Prometheus alerts
- [ ] Index links to all four runbooks

---

## WP5.5 — Cost Tracking + SLO Alerts

### Files to Create/Update

| Action | Path |
|--------|------|
| CREATE | `internal/cost/aggregator.go` — Query `llm_usage` by agent_id, model, day; expose metrics or optional HTTP endpoint |
| CREATE | `internal/cost/aggregator_test.go` — Unit tests |
| UPDATE | `pkg/metrics/metrics.go` — Add `agent_id` label to `astra_llm_cost_dollars` and `astra_llm_token_usage_total` if not present; or add `astra_llm_cost_dollars_total{agent_id, model}` |
| CREATE/UPDATE | `cmd/cost-tracker/main.go` — Optional: standalone or embedded consumer that aggregates and exposes metrics |
| UPDATE | `deployments/prometheus/rules/astra-alerts.yaml` — Add SLO alerts: p99 read >10ms, scheduling median >50ms |
| UPDATE | `docs/runbooks/` — Reference SLO breach runbook steps |

### Acceptance

- [ ] Cost per agent/model/day visible (metrics or dashboard)
- [ ] SLO breach alerts defined (p99 read, scheduling latency)
- [ ] `llm_usage` table is source; no sync DB on hot path

---

## WP5.6 — Helm Hardening

### Files to Create/Update

| Action | Path |
|--------|------|
| UPDATE | `deployments/helm/astra/values.yaml` — Per-service replicaCount, resources, autoscaling; enable autoscaling for api-gateway, scheduler, execution-worker |
| CREATE | `deployments/helm/astra/templates/hpa.yaml` — HPA for api-gateway, scheduler-service, execution-worker (CPU or custom metric) |
| CREATE | `deployments/helm/astra/templates/pdb.yaml` — PodDisruptionBudget (minAvailable: 1 or maxUnavailable: 1) for critical deployments |
| UPDATE | `deployments/helm/astra/templates/deployment.yaml` — Ensure resource requests/limits on all containers; support multiple deployments if chart is multi-service |
| CREATE | `deployments/helm/astra/README.md` — Document sizing, HPA triggers, PDB semantics |
| UPDATE | `.github/workflows/ci.yml` — Add step: `helm template deployments/helm/astra` and `helm install --dry-run` (or equivalent) |

### Acceptance

- [ ] HPA and PDB templates render
- [ ] Resource limits set on all containers
- [ ] CI runs `helm template` successfully

---

## WP5.7 — Observability Docs & Consistency

### Files to Create/Update

| Action | Path |
|--------|------|
| CREATE | `docs/observability.md` — Stack: OTLP endpoint, Prometheus scrape, log aggregation; trace_id propagation; metrics list (astra_*); dashboard/runbook references |
| UPDATE | `pkg/metrics/metrics.go` — Add `astra_worker_heartbeat_total`, `astra_events_processed_total` if missing |
| AUDIT | `cmd/task-service/main.go`, `cmd/scheduler-service/main.go`, `cmd/execution-worker/main.go` — Emit trace spans and `astra_task_latency_seconds`, `astra_task_success_total`, `astra_task_failure_total`, `astra_scheduler_ready_queue_depth` |
| CREATE/UPDATE | `pkg/otel/otel.go` — Trace_id propagation (gRPC metadata, context); logs include trace_id |
| UPDATE | Services — Ensure logs and events include trace_id where applicable |

### Acceptance

- [ ] `docs/observability.md` exists and describes stack
- [ ] Task flow has trace spans and metrics
- [ ] Dashboards and runbooks reference observability doc

---

## scripts/validate.sh — Phase 5 Section

Replace the placeholder `skip_test` calls (lines 336–338) with:

```bash
# ═══════════════════════════════════════════════
# PHASE 5 — Scale & Hardening
# ═══════════════════════════════════════════════
echo ""
echo "$(bold '═══ PHASE 5: Scale & Hardening ═══')"

echo "Load test assets:"
assert_eq "load test README exists" "true" "$(test -f tests/load/README.md && echo true || echo false)"
assert_eq "load test harness exists" "true" "$(test -f tests/load/k6-config.js -o -f tests/load/main.go && echo true || echo false)"

echo "Grafana dashboards:"
assert_eq "cluster-overview dashboard exists" "true" "$(test -f deployments/grafana/dashboards/cluster-overview.json && echo true || echo false)"
assert_eq "agent-health dashboard exists" "true" "$(test -f deployments/grafana/dashboards/agent-health.json && echo true || echo false)"
assert_eq "cost dashboard exists" "true" "$(test -f deployments/grafana/dashboards/cost.json && echo true || echo false)"

echo "Prometheus alert rules:"
assert_eq "astra-alerts.yaml exists" "true" "$(test -f deployments/prometheus/rules/astra-alerts.yaml && echo true || echo false)"

echo "Runbooks:"
assert_eq "worker-lost runbook exists" "true" "$(test -f docs/runbooks/worker-lost.md && echo true || echo false)"
assert_eq "high-error-rate runbook exists" "true" "$(test -f docs/runbooks/high-error-rate.md && echo true || echo false)"
assert_eq "db-redis runbook exists" "true" "$(test -f docs/runbooks/db-redis.md && echo true || echo false)"
assert_eq "llm-cost-spike runbook exists" "true" "$(test -f docs/runbooks/llm-cost-spike.md && echo true || echo false)"
assert_eq "runbooks README exists" "true" "$(test -f docs/runbooks/README.md && echo true || echo false)"

echo "Helm hardening:"
assert_eq "HPA template exists" "true" "$(test -f deployments/helm/astra/templates/hpa.yaml && echo true || echo false)"
assert_eq "PDB template exists" "true" "$(test -f deployments/helm/astra/templates/pdb.yaml && echo true || echo false)"
HELM_TEMPLATE=$(helm template astra deployments/helm/astra 2>/dev/null && echo ok || echo fail)
assert_eq "helm template renders" "ok" "$HELM_TEMPLATE"

echo "Observability:"
assert_eq "docs/observability.md exists" "true" "$(test -f docs/observability.md && echo true || echo false)"

# Optional: if Prometheus reachable, verify metrics endpoint (skip if not available)
# METRICS=$(curl -sf http://localhost:9090/metrics 2>/dev/null | grep -c astra_ || echo "0")
# assert_eq "astra metrics exposed" "true" "$([ "$METRICS" -ge 1 ] && echo true || echo false)"
```

### Minimal Acceptance Checks (validate.sh)

| Check | Purpose |
|-------|---------|
| `tests/load/README.md` | Load test plan present |
| `tests/load/k6-config.js` or `tests/load/main.go` | Load harness present |
| `deployments/grafana/dashboards/*.json` | Three dashboards versioned |
| `deployments/prometheus/rules/astra-alerts.yaml` | Alert rules versioned |
| `docs/runbooks/*.md` | Four runbooks + index |
| `deployments/helm/astra/templates/hpa.yaml`, `pdb.yaml` | HPA and PDB in chart |
| `helm template` | Chart renders without error |
| `docs/observability.md` | Observability doc present |

---

## Order of Execution (Recommended)

1. **WP5.7** — Observability consistency first (metrics, docs, trace_id)
2. **WP5.2, WP5.3, WP5.4** — Dashboards, alerts, runbooks (parallel)
3. **WP5.5** — Cost tracking + SLO alerts
4. **WP5.1** — Load tests (after metrics/dashboards)
5. **WP5.6** — Helm hardening
6. **validate.sh** — Update last

---

## Out of Scope (In-Repo)

- **WP5.8** — Sharding; defer unless single-shard fails load test
- **External infra** — Prometheus/Grafana/OTLP must be provisioned separately or via docker-compose
- **Load test execution in CI** — Validate asset presence only; full load run is staging/manual
