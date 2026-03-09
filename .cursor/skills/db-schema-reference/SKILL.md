# Database Schema Reference

Quick reference for the Astra PostgreSQL schema. Full DDL defined in the PRD and migration files.

## Extensions

- `uuid-ossp` — UUID generation (`uuid_generate_v4()`)
- `vector` (pgvector) — Embedding vector storage and similarity search

## Core Tables

### `agents`
Agent lifecycle and configuration.
```sql
CREATE TABLE agents (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  name TEXT,
  status TEXT,
  config JSONB,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT now(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT now()
);
```

### `goals`
Goals assigned to agents.
```sql
CREATE TABLE goals (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  agent_id UUID REFERENCES agents(id),
  goal_text TEXT,
  priority INT DEFAULT 100,
  status TEXT,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT now()
);
-- idx_goals_agent ON goals(agent_id)
```

### `tasks`
Task nodes in DAG graphs.
```sql
CREATE TABLE tasks (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  graph_id UUID,
  goal_id UUID REFERENCES goals(id),
  agent_id UUID,
  type TEXT,
  status TEXT,
  payload JSONB,
  result JSONB,
  priority INT DEFAULT 100,
  retries INT DEFAULT 0,
  max_retries INT DEFAULT 5,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT now(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT now()
);
-- idx_tasks_agent ON tasks(agent_id)
-- idx_tasks_status ON tasks(status)
-- idx_tasks_graph ON tasks(graph_id)
-- CHECK (status IN ('pending','queued','scheduled','running','completed','failed'))
```

### `task_dependencies`
DAG edges between tasks.
```sql
CREATE TABLE task_dependencies (
  task_id UUID REFERENCES tasks(id),
  depends_on UUID REFERENCES tasks(id),
  PRIMARY KEY (task_id, depends_on)
);
-- idx_task_dep_dependson ON task_dependencies(depends_on)
```

### `memories`
Agent episodic/semantic memory with pgvector embeddings.
```sql
CREATE TABLE memories (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  agent_id UUID REFERENCES agents(id),
  memory_type TEXT,
  content TEXT,
  embedding VECTOR(1536),
  created_at TIMESTAMP WITH TIME ZONE DEFAULT now()
);
-- idx_memories_agent ON memories(agent_id)
-- idx_memory_embedding ON memories USING ivfflat (embedding)
```

### `artifacts`
Tool execution outputs.
```sql
CREATE TABLE artifacts (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  agent_id UUID,
  task_id UUID,
  uri TEXT,
  metadata JSONB,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT now()
);
```

### `events`
Event sourcing log — immutable audit trail.
```sql
CREATE TABLE events (
  id BIGSERIAL PRIMARY KEY,
  event_type TEXT,
  actor_id UUID,
  payload JSONB,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT now()
);
-- idx_events_actor ON events(actor_id)
```

### `workers`
Worker pool registration and heartbeat tracking.
```sql
CREATE TABLE workers (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  hostname TEXT,
  status TEXT,
  last_heartbeat TIMESTAMP WITH TIME ZONE
);
```

## Key Query Patterns

### Ready task detection (scheduler hot path)
```sql
SELECT t.id
FROM tasks t
WHERE t.status = 'pending'
  AND NOT EXISTS (
    SELECT 1 FROM task_dependencies d
    JOIN tasks td ON td.id = d.depends_on
    WHERE d.task_id = t.id AND td.status != 'completed'
  )
FOR UPDATE SKIP LOCKED;
```

### Task state transition (transactional)
```sql
BEGIN;
UPDATE tasks SET status = $1, updated_at = now() WHERE id = $2 AND status = $3;
INSERT INTO events (event_type, actor_id, payload) VALUES ($4, $5, $6);
COMMIT;
```

## Migration Files

```
0001_initial_schema.sql      — agents, tasks tables
0002_task_dependencies.sql   — task_dependencies table
0003_memories_pgvector.sql   — pgvector extension, memories table
0004_artifacts.sql           — artifacts table
0005_workers.sql             — workers table
0006_indexes.sql             — all indexes
0007_events.sql              — events table
0008_constraints.sql         — CHECK constraints on task status
```

## Conventions

- All primary keys are UUID
- All tables have `created_at` (some have `updated_at` with trigger)
- JSONB for flexible payloads (`config`, `payload`, `result`, `metadata`)
- `FOR UPDATE SKIP LOCKED` for concurrent task claiming
- Event sourcing: every state change appends to `events`
