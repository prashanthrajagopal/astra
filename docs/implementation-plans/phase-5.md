# Phase 5 — Scale & Production Hardening — Implementation Plan

**Depends on:** Phase 4 complete. Acceptance: full orchestration, eval, JWT/OPA, approval gates, and usage audit in place.

**PRD references:** §17 Observability, §19 Deployment & Scaling, §20 Failure Modes & Runbooks, §21 CI/CD & Testing, §22 Cost Management, §24 SLAs, §25 Phase 5.

---

## 1. Phase goal

Harden the system for production: load testing at target scale (10k agents, 1M tasks), observability dashboards (Grafana), alerting (Prometheus), runbooks, cost tracking, SLO enforcement (10ms reads, 50ms scheduling), and Helm chart improvements (HPA, PDB, resource limits).

---

## 2. Dependencies

- **Phase 4** complete: goal-service, planner, identity, access-control, evaluation, usage audit.
- **Infra:** Prometheus, Grafana, OpenTelemetry collector (or equivalent) available; Helm chart in deployments/helm/astra/.

---

## 3. Work packages

### WP5.1 — Load testing (10k agents, 1M tasks)

**Description:** Design and run load tests to validate system at PRD target scale. Scenarios: many agents creating goals; high task throughput; scheduler and worker pool under load. Measure: p99 API latency (≤10ms for reads), scheduling latency (median ≤50ms, P95 ≤500ms), task completion rate, error rate. PRD §24.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Define load test scenarios: e.g. N agents, each M goals, each goal K tasks; sustained and burst. | QA / Tech Lead | Test plan doc |
| 2 | Implement load test harness (e.g. k6, Gatling, or Go benchmark) that drives api-gateway and/or gRPC (create agent, create goal, poll task status). | QA Engineer | tests/load/ or scripts |
| 3 | Run tests against staging/pre-prod; collect metrics (latency percentiles, throughput, errors). | QA Engineer | Results and report |
| 4 | Identify bottlenecks (DB, Redis, scheduler tick, worker pool); document and file follow-up items. | QA / Go Engineer | Report |
| 5 | Target: 10k agents, 1M tasks/day (or equivalent); p99 read ≤10ms, scheduling median ≤50ms. | QA | Pass/fail criteria |

**Deliverables:** Load test suite; results report; bottleneck analysis.

**Acceptance criteria:** Load test runs; results documented; either targets met or remediation plan for gaps.

---

### WP5.2 — Grafana dashboards

**Description:** Create Grafana dashboards for cluster overview, agent health, and cost. Use Prometheus metrics from §17: task latency, success/failure counts, actor count, worker heartbeats, LLM token usage and cost, scheduler queue depth. Dashboards: Cluster Overview, Agent Health, Cost (LLM per agent/model/day).

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Dashboard: Cluster Overview — capacity, active agents, task throughput, error rate, queue depth. | DevOps / QA | Grafana JSON or provisioning |
| 2 | Dashboard: Agent Health — per-agent throughput, avg latency, failure rate. | DevOps / QA | Same |
| 3 | Dashboard: Cost — LLM token usage and cost per agent, per model, per day (from llm_usage or metrics). | DevOps / QA | Same |
| 4 | Store dashboards in repo (deployments/grafana/dashboards/ or similar); document how to import. | DevOps | README |

**Deliverables:** At least three Grafana dashboards; versioned in repo.

**Acceptance criteria:** Dashboards load and show live data from Prometheus; documented.

---

### WP5.3 — Prometheus alerting rules

**Description:** Define alerting rules per PRD §17: high task failure rate (>5% over 5min), high queue depth (>10k pending), low worker availability (<50% registered), LLM cost spike (>2x daily average). Configure Prometheus to evaluate rules and route to Alertmanager (or equivalent).

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Alert rules: task failure rate, queue depth, worker availability, LLM cost spike. | DevOps Engineer | deployments/prometheus/rules/ or values |
| 2 | Document severity and runbook links for each alert. | DevOps | docs/ or annotations |
| 3 | Integration: Alertmanager (or PagerDuty/Slack) if in scope; else document for operator. | DevOps | Config or doc |

**Deliverables:** Prometheus alert rules; documentation.

**Acceptance criteria:** Alerts fire under defined conditions; runbook references in place.

---

### WP5.4 — Runbooks (docs/runbooks/)

**Description:** Write runbooks for failure modes per PRD §20: Worker Lost, High Error Rate, Postgres/Redis issues, scheduler imbalance, LLM cost spike. Each runbook: steps to detect, triage, contain, remediate. Reference Grafana and Prometheus.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Runbook: Worker Lost — identify last heartbeat, check stream, move in-flight tasks to queued, restart worker. | SRE / DevOps | docs/runbooks/worker-lost.md |
| 2 | Runbook: High Error Rate — sample failed tasks, check traces, pause intake if needed, rollback if code. | SRE / DevOps | docs/runbooks/high-error-rate.md |
| 3 | Runbook: Postgres/Redis connectivity and failover. | SRE / DevOps | docs/runbooks/db-redis.md |
| 4 | Runbook: LLM cost spike — disable premium routing, enforce lower tiers. | SRE / DevOps | docs/runbooks/llm-cost-spike.md |
| 5 | Index or README listing all runbooks. | DevOps | docs/runbooks/README.md |

**Deliverables:** At least four runbooks; index.

**Acceptance criteria:** Runbooks are actionable; tested in drill or documented as “to be tested.”

---

### WP5.5 — Cost tracking service and SLO enforcement

**Description:** Cost tracking: aggregate llm_usage by agent, model, day; expose via API or dashboard; alert when over budget. SLO enforcement: ensure 10ms read and 50ms scheduling are measurable and enforced (e.g. dashboards, alerts when SLO breached). PRD §22, §24.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Cost aggregation: query llm_usage (or stream consumer) to compute daily/weekly cost per agent and per model; expose metrics or API. | Go Engineer | internal/cost or cmd/cost-tracking (optional) |
| 2 | Prometheus metrics: astra_llm_cost_dollars, astra_llm_token_usage_total already in place; add aggregation by agent if missing. | Go Engineer | pkg/metrics or services |
| 3 | SLO alerts: p99 read latency >10ms, scheduling latency >50ms median; document in alerting and runbooks. | DevOps | Alerts + docs |
| 4 | Optional: cost_tracking service that writes aggregated cost to a table or cache for dashboard. | Go Engineer | Design or implementation |

**Deliverables:** Cost visibility (metrics/dashboard); SLO alerts.

**Acceptance criteria:** Cost per agent/model/day visible; SLO breaches trigger alerts.

---

### WP5.6 — Helm chart hardening (HPA, PDB, resource limits)

**Description:** Update Helm chart (deployments/helm/astra/): HorizontalPodAutoscaler for stateless services (api-gateway, scheduler, workers) based on CPU and/or request rate and queue depth; PodDisruptionBudget for graceful rollout; resource requests/limits on all containers; optional topology spread. PRD §19.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | HPA: api-gateway, scheduler-service, execution-worker (scale on CPU or custom metric e.g. queue depth). | DevOps Engineer | deployments/helm/astra/templates/ |
| 2 | PDB: minAvailable or maxUnavailable for critical deployments so evictions are bounded. | DevOps | Same |
| 3 | Resource limits: requests and limits for CPU/memory on every container; document sizing. | DevOps | values.yaml + docs |
| 4 | Optional: KEDA or similar for queue-depth-based scaling of workers. | DevOps | Same |
| 5 | Chart test: helm template and helm install --dry-run (or CI step). | DevOps | CI |

**Deliverables:** Updated Helm chart; CI validation.

**Acceptance criteria:** HPA and PDB in place; resource limits set; chart installs and scales.

---

### WP5.7 — Observability: tracing and metrics consistency

**Description:** Ensure tracing (OpenTelemetry) and metrics are consistent across services: each task execution has root span; trace_id in logs and events; metrics astra_task_latency_seconds, astra_task_success_total, astra_task_failure_total, astra_scheduler_ready_queue_depth populated. PRD §17.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Audit services: task-service, scheduler, execution-worker emit trace spans and metrics per PRD. | Go Engineer | Code review + fixes |
| 2 | Ensure trace_id propagated (gRPC metadata, context); logs include trace_id. | Go Engineer | pkg/otel or interceptors |
| 3 | Metrics: register and increment in hot paths; verify in Grafana. | Go Engineer | Same |
| 4 | Document observability stack (OTLP endpoint, Prometheus scrape, log aggregation). | DevOps | docs/observability.md |

**Deliverables:** Tracing and metrics verified; doc.

**Acceptance criteria:** Traces and metrics present for task flow; dashboard and runbooks reference them.

---

### WP5.8 — Sharding and multi-scheduler (optional)

**Description:** If not done earlier: scheduler sharding by agent_id or graph_id (hash % shard_count); multiple scheduler instances each owning a subset of shards; stream name astra:tasks:shard:<n>. PRD §8. Phase 5 can formalize shard assignment and rebalancing.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Scheduler: compute shard from graph_id or agent_id; publish to astra:tasks:shard:<n>. | Go Engineer | internal/scheduler |
| 2 | Shard assignment: each scheduler instance claims shards (e.g. via Postgres or Redis); document rebalance. | Go Engineer | Design + impl |
| 3 | Workers consume from all shards or shard-assigned; document. | Go Engineer | Same |
| 4 | Load test with multiple schedulers. | QA | Tests |

**Deliverables:** Sharding implementation; tests. Optional if single shard suffices for target load.

**Acceptance criteria:** Multiple scheduler replicas can run without double-dispatch; shards balanced.

---

## 4. Delegation hints

| Work package | Primary owner | Hand-off |
|--------------|---------------|----------|
| WP5.1 load test | QA Engineer | Report to Tech Lead; Go Engineer for fixes |
| WP5.2 dashboards | DevOps Engineer | Use metrics from services |
| WP5.3 alerting | DevOps Engineer | Link to runbooks |
| WP5.4 runbooks | SRE / DevOps | Reference dashboards and alerts |
| WP5.5 cost & SLO | Go Engineer + DevOps | Metrics and alerts |
| WP5.6 Helm | DevOps Engineer | CI and deploy test |
| WP5.7 observability | Go Engineer | DevOps for collector/config |
| WP5.8 sharding | Go Engineer | Optional; QA load test |

---

## 5. Ordering within Phase 5

1. **Early:** WP5.7 (observability consistency) so load test and dashboards have data.
2. **Parallel:** WP5.2 (dashboards), WP5.3 (alerting), WP5.4 (runbooks).
3. **Then:** WP5.1 (load test) once metrics and tracing are in place; WP5.5 (cost & SLO) in parallel.
4. **Then:** WP5.6 (Helm) and WP5.8 (sharding if in scope).

---

## 6. Risks / open decisions

- **Load test environment:** Staging vs dedicated cluster; data size (10k agents, 1M tasks) may require sustained run or synthetic data. Confirm with DevOps.
- **Sharding necessity:** If single-shard meets 1M tasks/day, WP5.8 can be deferred; document decision.
- **mTLS (S1):** Phase 5 may add mTLS between services for production; document in security checklist.
- **Cost tracking scope:** Full “cost_tracking service” vs metrics-only; align with product.

---

## Sign-off (Phase 5 complete)

- [ ] Load test run at target scale; results documented; SLOs met or remediation planned.
- [ ] Grafana dashboards (Cluster, Agent Health, Cost) operational.
- [ ] Prometheus alert rules and runbooks in place and referenced.
- [ ] Helm chart has HPA, PDB, resource limits; chart validated in CI.
- [ ] Cost and SLO visibility and alerts operational.
- [ ] Tracing and metrics consistent; observability doc updated.
