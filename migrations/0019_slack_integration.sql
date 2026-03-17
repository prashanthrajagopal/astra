-- Migration 0019: Slack integration
-- Tables for connecting Astra chat agents to Slack (one workspace per org, channel bindings, user mapping, platform app config).
-- Idempotent: IF NOT EXISTS throughout.

-- ═══════════════════════════════════════════════════════════════════════════
-- PLATFORM: Slack app config (super-admin UI–entered secrets)
-- ═══════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS slack_app_config (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  key TEXT NOT NULL UNIQUE,
  value_encrypted TEXT,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

DROP TRIGGER IF EXISTS slack_app_config_updated_at ON slack_app_config;
CREATE TRIGGER slack_app_config_updated_at BEFORE UPDATE ON slack_app_config
  FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE INDEX IF NOT EXISTS idx_slack_app_config_key ON slack_app_config(key);

-- ═══════════════════════════════════════════════════════════════════════════
-- ORG-SCOPED: Slack workspace → org linkage
-- ═══════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS slack_workspaces (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  slack_workspace_id TEXT NOT NULL,
  bot_token_ref TEXT,
  default_agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(slack_workspace_id)
);

DROP TRIGGER IF EXISTS slack_workspaces_updated_at ON slack_workspaces;
CREATE TRIGGER slack_workspaces_updated_at BEFORE UPDATE ON slack_workspaces
  FOR EACH ROW EXECUTE FUNCTION update_updated_at();

DO $$ BEGIN IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'slack_workspaces' AND column_name = 'org_id') THEN CREATE INDEX IF NOT EXISTS idx_slack_workspaces_org ON slack_workspaces(org_id); END IF; END $$;
CREATE INDEX IF NOT EXISTS idx_slack_workspaces_slack_id ON slack_workspaces(slack_workspace_id);

-- ═══════════════════════════════════════════════════════════════════════════
-- Per-channel agent binding (optional)
-- ═══════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS slack_channel_bindings (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  slack_channel_id TEXT NOT NULL,
  agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(org_id, slack_channel_id)
);

DO $$ BEGIN IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'slack_channel_bindings' AND column_name = 'org_id') THEN CREATE INDEX IF NOT EXISTS idx_slack_channel_bindings_org ON slack_channel_bindings(org_id); END IF; END $$;
DO $$ BEGIN IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'slack_channel_bindings' AND column_name = 'org_id') THEN CREATE INDEX IF NOT EXISTS idx_slack_channel_bindings_channel ON slack_channel_bindings(org_id, slack_channel_id); END IF; END $$;

-- ═══════════════════════════════════════════════════════════════════════════
-- Slack user → Astra user mapping (per org)
-- ═══════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS slack_user_mappings (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  slack_user_id TEXT NOT NULL,
  astra_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(org_id, slack_user_id)
);

DO $$ BEGIN IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'slack_user_mappings' AND column_name = 'org_id') THEN CREATE INDEX IF NOT EXISTS idx_slack_user_mappings_org ON slack_user_mappings(org_id); END IF; END $$;
DO $$ BEGIN IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'slack_user_mappings' AND column_name = 'org_id') THEN CREATE INDEX IF NOT EXISTS idx_slack_user_mappings_slack_user ON slack_user_mappings(org_id, slack_user_id); END IF; END $$;

-- ═══════════════════════════════════════════════════════════════════════════
-- Optional: link chat session to Slack thread for continuity
-- ═══════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS slack_sessions (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  chat_session_id UUID NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
  org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  slack_workspace_id TEXT NOT NULL,
  slack_channel_id TEXT NOT NULL,
  slack_user_id TEXT NOT NULL,
  slack_thread_ts TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(slack_workspace_id, slack_channel_id, slack_user_id, slack_thread_ts)
);

CREATE INDEX IF NOT EXISTS idx_slack_sessions_chat ON slack_sessions(chat_session_id);
CREATE INDEX IF NOT EXISTS idx_slack_sessions_lookup ON slack_sessions(slack_workspace_id, slack_channel_id, slack_user_id);
