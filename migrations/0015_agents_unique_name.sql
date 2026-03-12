-- Enforce unique agent names: de-duplicate existing rows, then add UNIQUE(agents.name).
-- Keeps one row per name (oldest by created_at, then id); reassigns FKs from duplicates to the kept row.

-- Step 1: Build mapping of duplicate agent ids -> keeper id (one per name, oldest by created_at then id)
CREATE TEMP TABLE IF NOT EXISTS _agent_dedup (keeper_id UUID, dupe_id UUID);

INSERT INTO _agent_dedup (keeper_id, dupe_id)
WITH ranked AS (
  SELECT id, name,
         row_number() OVER (PARTITION BY name ORDER BY created_at ASC, id ASC) AS rn
  FROM agents
)
SELECT
  (SELECT id FROM agents a2 WHERE a2.name = r.name ORDER BY created_at ASC, id ASC LIMIT 1) AS keeper_id,
  r.id AS dupe_id
FROM ranked r
WHERE r.rn > 1;

-- Step 2: Reassign all FKs and agent_id columns from duplicate ids to keeper (no-op if no duplicates)
UPDATE goals g
SET agent_id = d.keeper_id
FROM _agent_dedup d
WHERE g.agent_id = d.dupe_id;

UPDATE tasks t
SET agent_id = d.keeper_id
FROM _agent_dedup d
WHERE t.agent_id = d.dupe_id;

UPDATE memories m
SET agent_id = d.keeper_id
FROM _agent_dedup d
WHERE m.agent_id = d.dupe_id;

UPDATE artifacts ar
SET agent_id = d.keeper_id
FROM _agent_dedup d
WHERE ar.agent_id = d.dupe_id;

UPDATE agent_documents ad
SET agent_id = d.keeper_id
FROM _agent_dedup d
WHERE ad.agent_id = d.dupe_id;

UPDATE phase_runs pr
SET agent_id = d.keeper_id
FROM _agent_dedup d
WHERE pr.agent_id = d.dupe_id;

UPDATE llm_usage lu
SET agent_id = d.keeper_id
FROM _agent_dedup d
WHERE lu.agent_id = d.dupe_id;

-- Step 3: Remove duplicate agent rows
DELETE FROM agents
WHERE id IN (SELECT dupe_id FROM _agent_dedup);

DROP TABLE IF EXISTS _agent_dedup;

-- Step 4: Enforce unique names going forward
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'agents_name_key') THEN
    ALTER TABLE agents ADD CONSTRAINT agents_name_key UNIQUE (name);
  END IF;
END $$;
