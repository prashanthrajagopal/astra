-- Migration 0023: Remove multi-tenant (LTI) feature
-- Drops organizations, teams, org/team memberships, agent_collaborators, agent_admins,
-- and all org_id, owner_id, team_id, visibility columns. Single-platform only.
-- Idempotent: safe to run multiple times (IF EXISTS, DO blocks).

-- ═══════════════════════════════════════════════════════════════════════════
-- 1. Drop foreign key constraints that reference organizations or teams
--    (so we can drop org/team tables and columns safely)
-- ═══════════════════════════════════════════════════════════════════════════

-- agents: drop FKs for org_id, owner_id, team_id (constraint names may vary; use standard pg naming)
DO $$ BEGIN
  ALTER TABLE agents DROP CONSTRAINT IF EXISTS agents_org_id_fkey;
EXCEPTION WHEN undefined_object THEN NULL;
END $$;
DO $$ BEGIN
  ALTER TABLE agents DROP CONSTRAINT IF EXISTS agents_owner_id_fkey;
EXCEPTION WHEN undefined_object THEN NULL;
END $$;
DO $$ BEGIN
  ALTER TABLE agents DROP CONSTRAINT IF EXISTS agents_team_id_fkey;
EXCEPTION WHEN undefined_object THEN NULL;
END $$;

-- goals
DO $$ BEGIN
  ALTER TABLE goals DROP CONSTRAINT IF EXISTS goals_org_id_fkey;
EXCEPTION WHEN undefined_object THEN NULL;
END $$;
DO $$ BEGIN
  ALTER TABLE goals DROP CONSTRAINT IF EXISTS goals_user_id_fkey;
EXCEPTION WHEN undefined_object THEN NULL;
END $$;

-- tasks, workers
DO $$ BEGIN
  ALTER TABLE tasks DROP CONSTRAINT IF EXISTS tasks_org_id_fkey;
EXCEPTION WHEN undefined_object THEN NULL;
END $$;
DO $$ BEGIN
  ALTER TABLE workers DROP CONSTRAINT IF EXISTS workers_org_id_fkey;
EXCEPTION WHEN undefined_object THEN NULL;
END $$;

-- slack_workspaces, slack_channel_bindings, slack_user_mappings, slack_sessions
DO $$ BEGIN
  ALTER TABLE slack_workspaces DROP CONSTRAINT IF EXISTS slack_workspaces_org_id_fkey;
EXCEPTION WHEN undefined_object THEN NULL;
END $$;
DO $$ BEGIN
  ALTER TABLE slack_channel_bindings DROP CONSTRAINT IF EXISTS slack_channel_bindings_org_id_fkey;
EXCEPTION WHEN undefined_object THEN NULL;
END $$;
DO $$ BEGIN
  ALTER TABLE slack_user_mappings DROP CONSTRAINT IF EXISTS slack_user_mappings_org_id_fkey;
EXCEPTION WHEN undefined_object THEN NULL;
END $$;
DO $$ BEGIN
  ALTER TABLE slack_sessions DROP CONSTRAINT IF EXISTS slack_sessions_org_id_fkey;
EXCEPTION WHEN undefined_object THEN NULL;
END $$;

-- Drop check constraint on agents.visibility
DO $$ BEGIN
  ALTER TABLE agents DROP CONSTRAINT IF EXISTS agents_valid_visibility;
EXCEPTION WHEN undefined_object THEN NULL;
END $$;

-- ═══════════════════════════════════════════════════════════════════════════
-- 2. Drop tables that depend on organizations/teams (dependency order)
-- ═══════════════════════════════════════════════════════════════════════════

DROP TABLE IF EXISTS agent_collaborators;
DROP TABLE IF EXISTS agent_admins;
DROP TABLE IF EXISTS team_memberships;
DROP TABLE IF EXISTS teams;
DROP TABLE IF EXISTS org_memberships;
DROP TABLE IF EXISTS organizations;

-- ═══════════════════════════════════════════════════════════════════════════
-- 3. Drop indexes on columns we are about to drop (optional; DROP COLUMN
--    drops them, but explicit for idempotency if run in partial state)
-- ═══════════════════════════════════════════════════════════════════════════

DROP INDEX IF EXISTS idx_agents_org;
DROP INDEX IF EXISTS idx_agents_owner;
DROP INDEX IF EXISTS idx_agents_visibility;
DROP INDEX IF EXISTS idx_goals_org;
DROP INDEX IF EXISTS idx_tasks_org;
DROP INDEX IF EXISTS idx_workers_org;
DROP INDEX IF EXISTS idx_events_org;
DROP INDEX IF EXISTS idx_llm_usage_org;
DROP INDEX IF EXISTS idx_approval_requests_org;
DROP INDEX IF EXISTS idx_chat_sessions_org;
DROP INDEX IF EXISTS idx_slack_workspaces_org;
DROP INDEX IF EXISTS idx_slack_channel_bindings_org;
DROP INDEX IF EXISTS idx_slack_channel_bindings_channel;
DROP INDEX IF EXISTS idx_slack_user_mappings_org;
DROP INDEX IF EXISTS idx_slack_user_mappings_slack_user;

-- ═══════════════════════════════════════════════════════════════════════════
-- 4. Drop org_id / owner_id / team_id / visibility columns
-- ═══════════════════════════════════════════════════════════════════════════

ALTER TABLE agents DROP COLUMN IF EXISTS org_id;
ALTER TABLE agents DROP COLUMN IF EXISTS owner_id;
ALTER TABLE agents DROP COLUMN IF EXISTS team_id;
ALTER TABLE agents DROP COLUMN IF EXISTS visibility;

ALTER TABLE goals DROP COLUMN IF EXISTS org_id;
ALTER TABLE goals DROP COLUMN IF EXISTS user_id;

ALTER TABLE tasks DROP COLUMN IF EXISTS org_id;
ALTER TABLE workers DROP COLUMN IF EXISTS org_id;
ALTER TABLE events DROP COLUMN IF EXISTS org_id;
ALTER TABLE memories DROP COLUMN IF EXISTS org_id;
ALTER TABLE llm_usage DROP COLUMN IF EXISTS org_id;
ALTER TABLE approval_requests DROP COLUMN IF EXISTS org_id;
ALTER TABLE chat_sessions DROP COLUMN IF EXISTS org_id;

ALTER TABLE slack_workspaces DROP COLUMN IF EXISTS org_id;
ALTER TABLE slack_channel_bindings DROP COLUMN IF EXISTS org_id;
ALTER TABLE slack_user_mappings DROP COLUMN IF EXISTS org_id;
ALTER TABLE slack_sessions DROP COLUMN IF EXISTS org_id;
