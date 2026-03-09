-- Phase 3 (WP3.6): prompt-manager — store prompt templates by name and version.
-- Used for GetPrompt(name, version) and list-by-name lookups.

CREATE TABLE IF NOT EXISTS prompts (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  name TEXT NOT NULL,
  version TEXT NOT NULL,
  body TEXT NOT NULL,
  variables_schema JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'prompts_name_version_key') THEN
    ALTER TABLE prompts ADD CONSTRAINT prompts_name_version_key UNIQUE (name, version);
  END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_prompts_name ON prompts(name);

DROP TRIGGER IF EXISTS prompts_updated_at ON prompts;
CREATE TRIGGER prompts_updated_at BEFORE UPDATE ON prompts
  FOR EACH ROW EXECUTE FUNCTION update_updated_at();
