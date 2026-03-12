-- Migration 0018: Multi-Tenancy
-- Converts Astra from single-tenant to multi-tenant with organizations, teams,
-- users, roles, and tiered agent visibility (global/public/team/private).
-- Idempotent: uses IF NOT EXISTS / IF NOT EXISTS throughout.

-- ═══════════════════════════════════════════════════════════════════════════
-- NEW TABLES
-- ═══════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS users (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  email TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  is_super_admin BOOLEAN NOT NULL DEFAULT false,
  last_login_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

DO $$ BEGIN
  ALTER TABLE users ADD CONSTRAINT users_valid_status
    CHECK (status IN ('active', 'suspended', 'deactivated'));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DROP TRIGGER IF EXISTS users_updated_at ON users;
CREATE TRIGGER users_updated_at BEFORE UPDATE ON users
  FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- Organizations (tenants)
CREATE TABLE IF NOT EXISTS organizations (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  name TEXT NOT NULL UNIQUE,
  slug TEXT NOT NULL UNIQUE,
  status TEXT NOT NULL DEFAULT 'active',
  config JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

DO $$ BEGIN
  ALTER TABLE organizations ADD CONSTRAINT orgs_valid_status
    CHECK (status IN ('active', 'suspended', 'archived'));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DROP TRIGGER IF EXISTS organizations_updated_at ON organizations;
CREATE TRIGGER organizations_updated_at BEFORE UPDATE ON organizations
  FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- User-org membership with role
CREATE TABLE IF NOT EXISTS org_memberships (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  role TEXT NOT NULL DEFAULT 'member',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(user_id, org_id)
);

DO $$ BEGIN
  ALTER TABLE org_memberships ADD CONSTRAINT org_memberships_valid_role
    CHECK (role IN ('admin', 'member'));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Teams within orgs
CREATE TABLE IF NOT EXISTS teams (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  slug TEXT NOT NULL,
  description TEXT DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(org_id, slug)
);

-- User-team membership
CREATE TABLE IF NOT EXISTS team_memberships (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role TEXT NOT NULL DEFAULT 'member',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(team_id, user_id)
);

DO $$ BEGIN
  ALTER TABLE team_memberships ADD CONSTRAINT team_memberships_valid_role
    CHECK (role IN ('admin', 'member'));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Agent collaborator grants (user or team)
CREATE TABLE IF NOT EXISTS agent_collaborators (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
  collaborator_type TEXT NOT NULL,
  collaborator_id UUID NOT NULL,
  permission TEXT NOT NULL DEFAULT 'use',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(agent_id, collaborator_type, collaborator_id)
);

DO $$ BEGIN
  ALTER TABLE agent_collaborators ADD CONSTRAINT ac_valid_type
    CHECK (collaborator_type IN ('user', 'team'));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
  ALTER TABLE agent_collaborators ADD CONSTRAINT ac_valid_perm
    CHECK (permission IN ('use', 'edit', 'admin'));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Agent admins (receive approve/reject requests)
CREATE TABLE IF NOT EXISTS agent_admins (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(agent_id, user_id)
);

-- ═══════════════════════════════════════════════════════════════════════════
-- ALTER EXISTING TABLES
-- ═══════════════════════════════════════════════════════════════════════════

-- agents: tenant columns
ALTER TABLE agents ADD COLUMN IF NOT EXISTS org_id UUID REFERENCES organizations(id) ON DELETE CASCADE;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS owner_id UUID REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS team_id UUID REFERENCES teams(id) ON DELETE SET NULL;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS visibility TEXT NOT NULL DEFAULT 'private';

DO $$ BEGIN
  ALTER TABLE agents ADD CONSTRAINT agents_valid_visibility
    CHECK (visibility IN ('global', 'public', 'team', 'private'));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Mark all existing agents (pre-multi-tenant) as global
UPDATE agents SET visibility = 'global' WHERE org_id IS NULL AND visibility = 'private';

-- goals
ALTER TABLE goals ADD COLUMN IF NOT EXISTS org_id UUID REFERENCES organizations(id) ON DELETE CASCADE;
ALTER TABLE goals ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id) ON DELETE SET NULL;

-- tasks
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS org_id UUID REFERENCES organizations(id) ON DELETE CASCADE;

-- workers
ALTER TABLE workers ADD COLUMN IF NOT EXISTS org_id UUID REFERENCES organizations(id) ON DELETE CASCADE;

-- events
ALTER TABLE events ADD COLUMN IF NOT EXISTS org_id UUID;

-- memories
ALTER TABLE memories ADD COLUMN IF NOT EXISTS org_id UUID;

-- llm_usage
ALTER TABLE llm_usage ADD COLUMN IF NOT EXISTS org_id UUID;
ALTER TABLE llm_usage ADD COLUMN IF NOT EXISTS user_id UUID;

-- approval_requests
ALTER TABLE approval_requests ADD COLUMN IF NOT EXISTS org_id UUID;
ALTER TABLE approval_requests ADD COLUMN IF NOT EXISTS requested_by UUID REFERENCES users(id);
ALTER TABLE approval_requests ADD COLUMN IF NOT EXISTS assigned_to UUID REFERENCES users(id);

-- chat_sessions
ALTER TABLE chat_sessions ADD COLUMN IF NOT EXISTS org_id UUID;

-- ═══════════════════════════════════════════════════════════════════════════
-- INDEXES
-- ═══════════════════════════════════════════════════════════════════════════

CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);
CREATE INDEX IF NOT EXISTS idx_org_memberships_user ON org_memberships(user_id);
CREATE INDEX IF NOT EXISTS idx_org_memberships_org ON org_memberships(org_id);
CREATE INDEX IF NOT EXISTS idx_teams_org ON teams(org_id);
CREATE INDEX IF NOT EXISTS idx_team_memberships_team ON team_memberships(team_id);
CREATE INDEX IF NOT EXISTS idx_team_memberships_user ON team_memberships(user_id);
CREATE INDEX IF NOT EXISTS idx_agents_org ON agents(org_id);
CREATE INDEX IF NOT EXISTS idx_agents_owner ON agents(owner_id);
CREATE INDEX IF NOT EXISTS idx_agents_visibility ON agents(visibility);
CREATE INDEX IF NOT EXISTS idx_goals_org ON goals(org_id);
CREATE INDEX IF NOT EXISTS idx_tasks_org ON tasks(org_id);
CREATE INDEX IF NOT EXISTS idx_workers_org ON workers(org_id);
CREATE INDEX IF NOT EXISTS idx_events_org ON events(org_id);
CREATE INDEX IF NOT EXISTS idx_llm_usage_org ON llm_usage(org_id);
CREATE INDEX IF NOT EXISTS idx_approval_requests_org ON approval_requests(org_id);
CREATE INDEX IF NOT EXISTS idx_chat_sessions_org ON chat_sessions(org_id);
CREATE INDEX IF NOT EXISTS idx_agent_collaborators_agent ON agent_collaborators(agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_admins_agent ON agent_admins(agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_admins_user ON agent_admins(user_id);
