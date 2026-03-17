-- Agent platform hardening: config revisions, tool registry, drain/quotas, memory TTL, allowlist

CREATE TABLE IF NOT EXISTS agent_config_revisions (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
  revision INT NOT NULL,
  payload JSONB NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_by TEXT,
  UNIQUE(agent_id, revision)
);
CREATE INDEX IF NOT EXISTS idx_agent_config_revisions_agent ON agent_config_revisions(agent_id, revision DESC);

ALTER TABLE agents ADD COLUMN IF NOT EXISTS active_config_revision INT;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS drain_mode BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS drain_started_at TIMESTAMPTZ;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS max_concurrent_goals INT;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS daily_token_budget BIGINT;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS priority SMALLINT NOT NULL DEFAULT 0;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS allowed_tools JSONB;

CREATE TABLE IF NOT EXISTS tool_definitions (
  name TEXT NOT NULL,
  version TEXT NOT NULL,
  risk_tier TEXT NOT NULL DEFAULT 'low',
  sandbox BOOLEAN NOT NULL DEFAULT false,
  description TEXT,
  metadata JSONB DEFAULT '{}',
  PRIMARY KEY (name, version)
);

INSERT INTO tool_definitions (name, version, risk_tier, sandbox, description) VALUES
  ('file_write', '1', 'medium', false, 'Write file under workspace'),
  ('file_read', '1', 'low', false, 'Read file from workspace'),
  ('shell_exec', '1', 'high', true, 'Execute shell in workspace'),
  ('list_files', '1', 'low', false, 'List workspace files'),
  ('code_generate', '1', 'medium', false, 'LLM codegen task'),
  ('browser:screenshot', '1', 'medium', true, 'Browser screenshot tool')
ON CONFLICT (name, version) DO NOTHING;

ALTER TABLE chat_sessions ADD COLUMN IF NOT EXISTS retention_days INT;
ALTER TABLE memories ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_memories_agent_expires ON memories(agent_id) WHERE expires_at IS NOT NULL;
