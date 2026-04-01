-- Add dual-approval fields to approval_requests for multi-approver gate support.
ALTER TABLE approval_requests ADD COLUMN IF NOT EXISTS required_approvals INT NOT NULL DEFAULT 1;
ALTER TABLE approval_requests ADD COLUMN IF NOT EXISTS approvals JSONB NOT NULL DEFAULT '[]';
