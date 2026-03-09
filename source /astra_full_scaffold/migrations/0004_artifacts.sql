
CREATE TABLE artifacts (
 id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
 task_id UUID,
 uri TEXT,
 metadata JSONB
);
