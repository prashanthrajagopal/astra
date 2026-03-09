
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE agents (
 id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
 name TEXT,
 status TEXT,
 config JSONB,
 created_at TIMESTAMP DEFAULT now()
);

CREATE TABLE tasks (
 id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
 agent_id UUID,
 type TEXT,
 status TEXT,
 payload JSONB,
 result JSONB,
 created_at TIMESTAMP DEFAULT now()
);
