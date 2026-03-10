-- Phase 9: Agent Profile & Context Management
-- Add system_prompt to agents for persona definition
ALTER TABLE agents ADD COLUMN IF NOT EXISTS system_prompt TEXT DEFAULT '';

-- Agent documents: rules, skills, context docs, and reference material attached to agents
CREATE TABLE IF NOT EXISTS agent_documents (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
  goal_id UUID REFERENCES goals(id) ON DELETE SET NULL,
  doc_type TEXT NOT NULL,
  name TEXT NOT NULL,
  content TEXT,
  uri TEXT,
  metadata JSONB DEFAULT '{}',
  priority INT NOT NULL DEFAULT 100,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'agent_documents_valid_doc_type') THEN
    ALTER TABLE agent_documents ADD CONSTRAINT agent_documents_valid_doc_type
      CHECK (doc_type IN ('rule', 'skill', 'context_doc', 'reference'));
  END IF;
END $$;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'agent_documents_content_or_uri') THEN
    ALTER TABLE agent_documents ADD CONSTRAINT agent_documents_content_or_uri
      CHECK (content IS NOT NULL OR uri IS NOT NULL);
  END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_agent_documents_agent ON agent_documents(agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_documents_goal ON agent_documents(goal_id);
CREATE INDEX IF NOT EXISTS idx_agent_documents_type ON agent_documents(agent_id, doc_type);

-- Reuse the update_updated_at() trigger function from earlier migrations
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_trigger WHERE tgname = 'agent_documents_updated_at'
  ) THEN
    CREATE TRIGGER agent_documents_updated_at BEFORE UPDATE ON agent_documents
      FOR EACH ROW EXECUTE FUNCTION update_updated_at();
  END IF;
END;
$$;
