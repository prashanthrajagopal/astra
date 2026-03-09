# DB notes: Phase 3 & Phase 4 schema

## Phase 4 — Existing schema (migration 0009)

Migration **0009_phase_history_and_usage.sql** already provides:

- **llm_usage**: `request_id`, `agent_id`, `task_id`, `model`, `tokens_in`, `tokens_out`, `latency_ms`, `cost_dollars`, `created_at` — sufficient for WP4.9 (LLM usage persistence).
- **phase_runs**: `goal_id`, `agent_id`, `status`, `started_at`, `ended_at`, `summary`, `timeline`, etc. — sufficient for goal-service phase lifecycle.
- **phase_summaries**: pgvector embeddings for semantic search over phase outcomes.
- **events**: `idx_events_created_at` for time-range audit queries.

No further DB changes are required for WP4.9 or goal-service phase lifecycle.
