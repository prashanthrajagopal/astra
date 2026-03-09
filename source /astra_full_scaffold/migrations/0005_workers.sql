
CREATE TABLE workers (
 id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
 hostname TEXT,
 status TEXT,
 last_heartbeat TIMESTAMP
);
