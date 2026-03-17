package prompt

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Prompt is a stored prompt template.
type Prompt struct {
	ID              uuid.UUID
	Name            string
	Version         string
	Body            string
	VariablesSchema []byte // JSON
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Store persists and retrieves prompts.
type Store struct {
	db *sql.DB
}

// NewStore returns a new Store backed by db.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// GetPrompt returns the prompt for the given name and version.
// Returns (nil, nil) if not found.
func (s *Store) GetPrompt(ctx context.Context, name, version string) (*Prompt, error) {
	var p Prompt
	var schema []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, version, body, variables_schema, created_at, updated_at
		 FROM prompts WHERE name = $1 AND version = $2`,
		name, version).Scan(
		&p.ID, &p.Name, &p.Version, &p.Body, &schema, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("prompt.GetPrompt: %w", err)
	}
	p.VariablesSchema = schema
	return &p, nil
}

// SavePrompt inserts or updates a prompt. Uses p.ID if set, otherwise generates a new UUID.
// ON CONFLICT (name, version) updates body, variables_schema, and updated_at.
func (s *Store) SavePrompt(ctx context.Context, p *Prompt) error {
	id := p.ID
	if id == uuid.Nil {
		id = uuid.New()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO prompts (id, name, version, body, variables_schema, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5::jsonb, now(), now())
		 ON CONFLICT (name, version) DO UPDATE SET
		   body = EXCLUDED.body,
		   variables_schema = EXCLUDED.variables_schema,
		   updated_at = now()`,
		id, p.Name, p.Version, p.Body, p.VariablesSchema)
	if err != nil {
		return fmt.Errorf("prompt.SavePrompt: %w", err)
	}
	return nil
}

// ListByName returns all prompts with the given name, ordered by version.
func (s *Store) ListByName(ctx context.Context, name string) ([]Prompt, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, version, body, variables_schema, created_at, updated_at
		 FROM prompts WHERE name = $1 ORDER BY version`,
		name)
	if err != nil {
		return nil, fmt.Errorf("prompt.ListByName: %w", err)
	}
	defer rows.Close()
	var out []Prompt
	for rows.Next() {
		var p Prompt
		var schema []byte
		if err := rows.Scan(&p.ID, &p.Name, &p.Version, &p.Body, &schema, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("prompt.ListByName: scan: %w", err)
		}
		p.VariablesSchema = schema
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("prompt.ListByName: %w", err)
	}
	return out, nil
}
