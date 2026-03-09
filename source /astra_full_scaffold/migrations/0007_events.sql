
CREATE TABLE events (
 id BIGSERIAL PRIMARY KEY,
 event_type TEXT,
 payload JSONB,
 created_at TIMESTAMP DEFAULT now()
);
