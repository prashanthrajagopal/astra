-- Phase 4 (WP4.8): tool execution approval gates (S6). Persist approval requests for human-in-the-loop.
-- Phase 4 schema note: Migration 0009 already provides llm_usage (request_id, agent_id, task_id, model,
-- tokens_in, tokens_out, latency_ms, cost_dollars, created_at) and phase_runs (goal_id, agent_id, status,
-- started_at, ended_at, summary, timeline, etc.) — no further DB changes required for WP4.9 or goal-service phase lifecycle.

CREATE TABLE IF NOT EXISTS approval_requests (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  task_id UUID REFERENCES tasks(id) ON DELETE SET NULL,
  worker_id UUID REFERENCES workers(id) ON DELETE SET NULL,
  tool_name TEXT,
  action_summary TEXT,
  status TEXT NOT NULL DEFAULT 'pending',
  requested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  decided_at TIMESTAMPTZ,
  decided_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'approval_requests_valid_status') THEN
    ALTER TABLE approval_requests ADD CONSTRAINT approval_requests_valid_status
      CHECK (status IN ('pending', 'approved', 'denied'));
  END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_approval_requests_status ON approval_requests(status);
CREATE INDEX IF NOT EXISTS idx_approval_requests_task ON approval_requests(task_id);
CREATE INDEX IF NOT EXISTS idx_approval_requests_worker ON approval_requests(worker_id);

DROP TRIGGER IF EXISTS approval_requests_updated_at ON approval_requests;
CREATE TRIGGER approval_requests_updated_at BEFORE UPDATE ON approval_requests
  FOR EACH ROW EXECUTE FUNCTION update_updated_at();
