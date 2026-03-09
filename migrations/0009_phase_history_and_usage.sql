-- Phase/build history, LLM usage, and audit support.
-- See docs/phase-history-usage-audit-design.md.

-- Phase runs: one row per phase (e.g. goal execution).
CREATE TABLE IF NOT EXISTS phase_runs (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  goal_id UUID REFERENCES goals(id) ON DELETE SET NULL,
  agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
  name TEXT,
  status TEXT NOT NULL DEFAULT 'running',
  started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ended_at TIMESTAMPTZ,
  summary TEXT,
  timeline JSONB DEFAULT '[]',
  log_file_path TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'phase_runs_valid_status') THEN
    ALTER TABLE phase_runs ADD CONSTRAINT phase_runs_valid_status
      CHECK (status IN ('running', 'completed', 'failed', 'cancelled'));
  END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_phase_runs_agent ON phase_runs(agent_id);
CREATE INDEX IF NOT EXISTS idx_phase_runs_goal ON phase_runs(goal_id);
CREATE INDEX IF NOT EXISTS idx_phase_runs_started ON phase_runs(started_at);

DROP TRIGGER IF EXISTS phase_runs_updated_at ON phase_runs;
CREATE TRIGGER phase_runs_updated_at BEFORE UPDATE ON phase_runs
  FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- Phase summaries: pgvector for semantic search over "what was done".
CREATE TABLE IF NOT EXISTS phase_summaries (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  phase_id UUID NOT NULL REFERENCES phase_runs(id) ON DELETE CASCADE,
  content TEXT NOT NULL,
  embedding VECTOR(1536),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_phase_summaries_phase ON phase_summaries(phase_id);
CREATE INDEX IF NOT EXISTS idx_phase_summary_embedding ON phase_summaries USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

-- LLM usage: one row per LLM request for analytics and audit.
CREATE TABLE IF NOT EXISTS llm_usage (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  request_id TEXT,
  agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
  task_id UUID REFERENCES tasks(id) ON DELETE SET NULL,
  model TEXT NOT NULL,
  tokens_in INT NOT NULL DEFAULT 0,
  tokens_out INT NOT NULL DEFAULT 0,
  latency_ms INT,
  cost_dollars NUMERIC(12,6),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_llm_usage_agent_created ON llm_usage(agent_id, created_at);
CREATE INDEX IF NOT EXISTS idx_llm_usage_task_created ON llm_usage(task_id, created_at);
CREATE INDEX IF NOT EXISTS idx_llm_usage_created ON llm_usage(created_at);

-- Audit: time-range queries on events.
CREATE INDEX IF NOT EXISTS idx_events_created_at ON events(created_at);
