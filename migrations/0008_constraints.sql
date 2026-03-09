DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'tasks_valid_status') THEN
    ALTER TABLE tasks ADD CONSTRAINT tasks_valid_status
      CHECK (status IN ('created', 'pending', 'queued', 'scheduled', 'running', 'completed', 'failed'));
  END IF;
END $$;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'agents_valid_status') THEN
    ALTER TABLE agents ADD CONSTRAINT agents_valid_status
      CHECK (status IN ('active', 'stopped', 'error'));
  END IF;
END $$;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'workers_valid_status') THEN
    ALTER TABLE workers ADD CONSTRAINT workers_valid_status
      CHECK (status IN ('active', 'draining', 'offline'));
  END IF;
END $$;

CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS agents_updated_at ON agents;
CREATE TRIGGER agents_updated_at BEFORE UPDATE ON agents
  FOR EACH ROW EXECUTE FUNCTION update_updated_at();

DROP TRIGGER IF EXISTS tasks_updated_at ON tasks;
CREATE TRIGGER tasks_updated_at BEFORE UPDATE ON tasks
  FOR EACH ROW EXECUTE FUNCTION update_updated_at();
