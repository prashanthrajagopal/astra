-- Migration 0020: Slack token rotation (refresh_token)
-- Add refresh_token_ref to slack_workspaces for 12-hour token refresh.

ALTER TABLE slack_workspaces ADD COLUMN IF NOT EXISTS refresh_token_ref TEXT;
