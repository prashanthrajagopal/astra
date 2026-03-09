
CREATE TABLE agents(
id UUID PRIMARY KEY,
name TEXT
);

CREATE TABLE tasks(
id UUID PRIMARY KEY,
agent_id UUID,
status TEXT,
payload JSONB
);
