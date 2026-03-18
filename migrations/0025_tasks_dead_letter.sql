-- Add dead_letter status to tasks for tasks that exceed max retries (P0.2).
-- Drop and re-add the CHECK constraint to include 'dead_letter'.
ALTER TABLE tasks DROP CONSTRAINT IF EXISTS tasks_valid_status;
ALTER TABLE tasks ADD CONSTRAINT tasks_valid_status
  CHECK (status IN ('created', 'pending', 'queued', 'scheduled', 'running', 'completed', 'failed', 'dead_letter'));
