-- result_payload: stores the final JSON execution plan produced by the last completed task
ALTER TABLE goals ADD COLUMN IF NOT EXISTS result_payload JSONB;

-- workspace_tag: optional label for workspace-scoped goals (e.g. "my-app", "la28-olympus")
ALTER TABLE goals ADD COLUMN IF NOT EXISTS workspace_tag TEXT;
CREATE INDEX IF NOT EXISTS idx_goals_workspace_tag ON goals(workspace_tag) WHERE workspace_tag IS NOT NULL;

-- http_fetch tool for agents to call external APIs from an allow-list
INSERT INTO tool_definitions (name, version, risk_tier, sandbox, description)
VALUES ('http_fetch', '1', 'medium', false, 'Authenticated HTTP GET/POST to allow-listed external URLs')
ON CONFLICT (name, version) DO NOTHING;
