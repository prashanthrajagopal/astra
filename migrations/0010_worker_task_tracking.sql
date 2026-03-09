DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='tasks' AND column_name='worker_id') THEN
    ALTER TABLE tasks ADD COLUMN worker_id UUID REFERENCES workers(id) ON DELETE SET NULL;
  END IF;
END $$;
CREATE INDEX IF NOT EXISTS idx_tasks_worker ON tasks(worker_id);
