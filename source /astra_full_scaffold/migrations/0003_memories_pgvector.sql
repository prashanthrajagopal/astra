
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE memories (
 id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
 agent_id UUID,
 content TEXT,
 embedding VECTOR(1536)
);
