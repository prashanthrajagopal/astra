-- Migration 0021: Slack proactive post default channel
-- Optional Slack channel ID (or user ID for DM) for proactive messages when channel_id is not provided.

ALTER TABLE slack_workspaces ADD COLUMN IF NOT EXISTS notification_channel_id TEXT;
