-- Per-agent external data source (each agent can listen to one external source: Redis Pub/Sub, GCP Pub/Sub, or WebSocket).
-- Optional: allow agent to send messages to Slack (e.g. when prompt instructs it); org must have Slack connected.
ALTER TABLE agents ADD COLUMN IF NOT EXISTS ingest_source_type TEXT;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS ingest_source_config JSONB;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS slack_notifications_enabled BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN agents.ingest_source_type IS 'External source type: redis_pubsub, gcp_pubsub, or websocket. Null = agent does not listen to an external source.';
COMMENT ON COLUMN agents.ingest_source_config IS 'Source-specific config, e.g. {"channel":"x"} for Redis, {"project":"p","subscription":"s"} for GCP, {"url":"wss://..."} for WebSocket.';
COMMENT ON COLUMN agents.slack_notifications_enabled IS 'When true, agent may post to Slack (e.g. when prompt instructs). Org must have Slack connected.';
