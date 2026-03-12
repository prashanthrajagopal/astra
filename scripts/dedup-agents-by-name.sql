-- One-off de-duplication of agents by name.
-- Keeps one row per agents.name (oldest by created_at, then id); reassigns all
-- references (goals, tasks, memories, artifacts, agent_documents, phase_runs, llm_usage)
-- from duplicate agent ids to the kept row, then deletes duplicate agents.
--
-- Usage: psql -f scripts/dedup-agents-by-name.sql $DATABASE_URL
-- Or:   psql $DATABASE_URL < scripts/dedup-agents-by-name.sql
--
-- Note: Migration 0015_agents_unique_name.sql does the same de-dup and adds
-- UNIQUE(agents.name). Run this script only if you need to de-dup without
-- applying the full migration (e.g. before adding the constraint yourself).

CREATE TEMP TABLE IF NOT EXISTS _agent_dedup (keeper_id UUID, dupe_id UUID) ON COMMIT DROP;

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

UPDATE goals g SET agent_id = d.keeper_id FROM _agent_dedup d WHERE g.agent_id = d.dupe_id;
UPDATE tasks t SET agent_id = d.keeper_id FROM _agent_dedup d WHERE t.agent_id = d.dupe_id;
UPDATE memories m SET agent_id = d.keeper_id FROM _agent_dedup d WHERE m.agent_id = d.dupe_id;
UPDATE artifacts ar SET agent_id = d.keeper_id FROM _agent_dedup d WHERE ar.agent_id = d.dupe_id;
UPDATE agent_documents ad SET agent_id = d.keeper_id FROM _agent_dedup d WHERE ad.agent_id = d.dupe_id;
UPDATE phase_runs pr SET agent_id = d.keeper_id FROM _agent_dedup d WHERE pr.agent_id = d.dupe_id;
UPDATE llm_usage lu SET agent_id = d.keeper_id FROM _agent_dedup d WHERE lu.agent_id = d.dupe_id;

DELETE FROM agents WHERE id IN (SELECT dupe_id FROM _agent_dedup);

DROP TABLE IF EXISTS _agent_dedup;
