package memory

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

const embeddingDim = 1536

type Memory struct {
	ID         uuid.UUID
	AgentID    uuid.UUID
	MemoryType string
	Content    string
	Embedding  []float32
}

type Store struct {
	db       *sql.DB
	embedder Embedder
}

// NewStore creates a memory store. If embedder is nil, Write with embedding nil
// does not compute an embedding; if non-nil, Write with embedding nil calls
// embedder.Embed and uses the result.
func NewStore(db *sql.DB, embedder Embedder) *Store {
	return &Store{db: db, embedder: embedder}
}

// Write inserts a memory. If embedding is nil and the store has an embedder,
// it calls embedder.Embed(content) and stores the result. If embedding is nil
// and there is no embedder, the embedding column is set to NULL.
func (s *Store) Write(ctx context.Context, agentID uuid.UUID, memType, content string, embedding []float32) (uuid.UUID, error) {
	id := uuid.New()

	if embedding == nil && s.embedder != nil {
		vec, err := s.embedder.Embed(ctx, content)
		if err != nil {
			return uuid.Nil, fmt.Errorf("memory.Write: embed: %w", err)
		}
		embedding = vec
	}

	if embedding == nil {
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO memories (id, agent_id, memory_type, content, created_at, embedding) VALUES ($1, $2, $3, $4, now(), NULL)`,
			id, agentID, memType, content)
		if err != nil {
			return uuid.Nil, fmt.Errorf("memory.Write: %w", err)
		}
		return id, nil
	}

	vecStr := formatVector(embedding)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO memories (id, agent_id, memory_type, content, created_at, embedding) VALUES ($1, $2, $3, $4, now(), $5::vector)`,
		id, agentID, memType, content, vecStr)
	if err != nil {
		return uuid.Nil, fmt.Errorf("memory.Write: %w", err)
	}
	return id, nil
}

// Search returns memories for the agent. If queryEmbedding is nil or not 1536-dimensional,
// results are ordered by created_at DESC. Otherwise results are ordered by cosine distance
// (embedding <=> queryEmbedding) and only rows with non-NULL embedding are considered.
func (s *Store) Search(ctx context.Context, agentID uuid.UUID, queryEmbedding []float32, topK int) ([]Memory, error) {
	if queryEmbedding == nil || len(queryEmbedding) != embeddingDim {
		return s.searchByCreatedAt(ctx, agentID, topK)
	}
	return s.searchByVector(ctx, agentID, queryEmbedding, topK)
}

func (s *Store) searchByCreatedAt(ctx context.Context, agentID uuid.UUID, topK int) ([]Memory, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, agent_id, memory_type, content, embedding FROM memories WHERE agent_id = $1
		 AND (expires_at IS NULL OR expires_at > now()) ORDER BY created_at DESC LIMIT $2`,
		agentID, topK)
	if err != nil {
		return nil, fmt.Errorf("memory.Search: %w", err)
	}
	defer rows.Close()
	return scanMemories(rows)
}

func (s *Store) searchByVector(ctx context.Context, agentID uuid.UUID, queryEmbedding []float32, topK int) ([]Memory, error) {
	vecStr := formatVector(queryEmbedding)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, agent_id, memory_type, content, embedding FROM memories WHERE agent_id = $1 AND embedding IS NOT NULL
		 AND (expires_at IS NULL OR expires_at > now()) ORDER BY embedding <=> $2::vector LIMIT $3`,
		agentID, vecStr, topK)
	if err != nil {
		return nil, fmt.Errorf("memory.Search: %w", err)
	}
	defer rows.Close()
	return scanMemories(rows)
}

func scanMemories(rows *sql.Rows) ([]Memory, error) {
	var results []Memory
	for rows.Next() {
		var m Memory
		var embedSQL sql.NullString
		if err := rows.Scan(&m.ID, &m.AgentID, &m.MemoryType, &m.Content, &embedSQL); err != nil {
			return nil, fmt.Errorf("memory.Search: scan: %w", err)
		}
		if embedSQL.Valid && embedSQL.String != "" {
			emb, err := parseVector(embedSQL.String)
			if err != nil {
				return nil, fmt.Errorf("memory.Search: parse embedding: %w", err)
			}
			m.Embedding = emb
		}
		results = append(results, m)
	}
	return results, rows.Err()
}

// GetByID returns a single memory by id, including embedding if present.
func (s *Store) GetByID(ctx context.Context, id uuid.UUID) (*Memory, error) {
	var m Memory
	var embedSQL sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, agent_id, memory_type, content, embedding FROM memories WHERE id = $1`,
		id).Scan(&m.ID, &m.AgentID, &m.MemoryType, &m.Content, &embedSQL)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("memory.GetByID: %w", err)
	}
	if embedSQL.Valid && embedSQL.String != "" {
		emb, err := parseVector(embedSQL.String)
		if err != nil {
			return nil, fmt.Errorf("memory.GetByID: parse embedding: %w", err)
		}
		m.Embedding = emb
	}
	return &m, nil
}
