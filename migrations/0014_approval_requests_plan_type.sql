-- Extend approval_requests per docs/approval-system-extension-spec.md §3 (plan vs risky_task approval types).
-- Idempotent: safe to run multiple times.

-- Add new columns (only if not present)
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'approval_requests' AND column_name = 'request_type'
  ) THEN
    ALTER TABLE approval_requests ADD COLUMN request_type TEXT NOT NULL DEFAULT 'risky_task';
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'approval_requests' AND column_name = 'goal_id'
  ) THEN
    ALTER TABLE approval_requests ADD COLUMN goal_id UUID NULL REFERENCES goals(id) ON DELETE SET NULL;
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'approval_requests' AND column_name = 'graph_id'
  ) THEN
    ALTER TABLE approval_requests ADD COLUMN graph_id UUID NULL;
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'approval_requests' AND column_name = 'plan_payload'
  ) THEN
    ALTER TABLE approval_requests ADD COLUMN plan_payload JSONB NULL;
  END IF;
END $$;

-- Backfill: treat existing or empty request_type as risky_task (before adding CHECK)
UPDATE approval_requests
SET request_type = 'risky_task'
WHERE request_type IS NULL OR request_type = '';

-- Constraint: request_type IN ('plan', 'risky_task') — only if not exists
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'approval_requests_valid_request_type') THEN
    ALTER TABLE approval_requests ADD CONSTRAINT approval_requests_valid_request_type
      CHECK (request_type IN ('plan', 'risky_task'));
  END IF;
END $$;

-- Index for filtering by request_type
CREATE INDEX IF NOT EXISTS idx_approval_requests_request_type ON approval_requests(request_type);
