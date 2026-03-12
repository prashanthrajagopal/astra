-- Add actor_type column to agents, backfill from name (which currently stores actor_type)
ALTER TABLE agents ADD COLUMN IF NOT EXISTS actor_type TEXT;
UPDATE agents SET actor_type = name WHERE actor_type IS NULL;
ALTER TABLE agents ALTER COLUMN actor_type SET NOT NULL;
ALTER TABLE agents ALTER COLUMN actor_type SET DEFAULT '';
