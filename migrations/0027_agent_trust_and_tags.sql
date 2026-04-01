-- Add trust score, tags, and metadata to agents for trust-tier scheduling and filtering.
ALTER TABLE agents ADD COLUMN IF NOT EXISTS trust_score FLOAT NOT NULL DEFAULT 0.5;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS tags TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE agents ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}';

CREATE INDEX IF NOT EXISTS idx_agents_tags ON agents USING GIN(tags);
