ALTER TABLE tasks ADD CONSTRAINT tasks_valid_status
  CHECK (status IN ('created', 'pending', 'queued', 'scheduled', 'running', 'completed', 'failed'));

ALTER TABLE agents ADD CONSTRAINT agents_valid_status
  CHECK (status IN ('active', 'stopped', 'error'));

ALTER TABLE workers ADD CONSTRAINT workers_valid_status
  CHECK (status IN ('active', 'draining', 'offline'));

CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER agents_updated_at BEFORE UPDATE ON agents
  FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER tasks_updated_at BEFORE UPDATE ON tasks
  FOR EACH ROW EXECUTE FUNCTION update_updated_at();
