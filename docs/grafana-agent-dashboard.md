# Grafana — agent-scoped metrics

Prometheus metrics to chart:

| Metric | Labels | Notes |
|--------|--------|-------|
| `astra_llm_completion_seconds` | `agent_id`, `model` | Histogram from llm-router |
| `astra_llm_cost_dollars_total` | `agent_id`, `model` | When cost > 0 |
| `astra_llm_token_usage_total` | `model`, `direction` | Router backend path |

**Saturation:** Redis key `astra:llm:inflight` vs env `ASTRA_LLM_MAX_INFLIGHT`. Dashboard API: `GET /superadmin/api/dashboard/llm-saturation` (superadmin JWT).

**Token budget (per agent, UTC day):** Redis `agent:{uuid}:tokens:YYYY-MM-DD` — compared to `agents.daily_token_budget` at goal admission.
