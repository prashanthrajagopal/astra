-- Add cascade/dependency fields to goals for multi-agent goal orchestration.
ALTER TABLE goals ADD COLUMN IF NOT EXISTS cascade_id UUID;
ALTER TABLE goals ADD COLUMN IF NOT EXISTS depends_on_goal_ids UUID[];
ALTER TABLE goals ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ;
ALTER TABLE goals ADD COLUMN IF NOT EXISTS source_agent_id UUID REFERENCES agents(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_goals_cascade_id ON goals(cascade_id);
CREATE INDEX IF NOT EXISTS idx_goals_source_agent_id ON goals(source_agent_id);
