package memory

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

type Memory struct {
	ID         uuid.UUID
	AgentID    uuid.UUID
	MemoryType string
	Content    string
	Embedding  []float32
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Write(ctx context.Context, agentID uuid.UUID, memType, content string) (uuid.UUID, error) {
	id := uuid.New()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO memories (id, agent_id, memory_type, content, created_at) VALUES ($1, $2, $3, $4, now())`,
		id, agentID, memType, content)
	if err != nil {
		return uuid.Nil, fmt.Errorf("memory.Write: %w", err)
	}
	return id, nil
}

func (s *Store) Search(ctx context.Context, agentID uuid.UUID, query string, topK int) ([]Memory, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, agent_id, memory_type, content FROM memories WHERE agent_id = $1 ORDER BY created_at DESC LIMIT $2`,
		agentID, topK)
	if err != nil {
		return nil, fmt.Errorf("memory.Search: %w", err)
	}
	defer rows.Close()

	var results []Memory
	for rows.Next() {
		var m Memory
		if err := rows.Scan(&m.ID, &m.AgentID, &m.MemoryType, &m.Content); err != nil {
			return nil, fmt.Errorf("memory.Search: scan: %w", err)
		}
		results = append(results, m)
	}
	return results, rows.Err()
}
