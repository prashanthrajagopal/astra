
ALTER TABLE tasks
ADD CONSTRAINT valid_status
CHECK (status IN ('pending','running','completed','failed'));
