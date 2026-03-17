package agentdocs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ConfigRevisionPayload is stored in agent_config_revisions.payload.
type ConfigRevisionPayload struct {
	SystemPrompt string          `json:"system_prompt,omitempty"`
	Config       json.RawMessage `json:"config,omitempty"`
}

type ConfigRevision struct {
	ID        uuid.UUID       `json:"id"`
	AgentID   uuid.UUID       `json:"agent_id"`
	Revision  int             `json:"revision"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
	CreatedBy string          `json:"created_by,omitempty"`
}

// SaveAgentRevision appends the next revision number for the agent.
func (s *Store) SaveAgentRevision(ctx context.Context, agentID uuid.UUID, payload json.RawMessage, createdBy string) (*ConfigRevision, error) {
	var next int
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(revision), 0) + 1 FROM agent_config_revisions WHERE agent_id = $1`,
		agentID).Scan(&next)
	if err != nil {
		return nil, fmt.Errorf("SaveAgentRevision next rev: %w", err)
	}
	id := uuid.New()
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO agent_config_revisions (id, agent_id, revision, payload, created_by) VALUES ($1, $2, $3, $4::jsonb, $5)`,
		id, agentID, next, payload, nullStr(createdBy))
	if err != nil {
		return nil, fmt.Errorf("SaveAgentRevision insert: %w", err)
	}
	return &ConfigRevision{ID: id, AgentID: agentID, Revision: next, Payload: payload, CreatedAt: time.Now().UTC(), CreatedBy: createdBy}, nil
}

func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// ListAgentRevisions returns revisions newest first.
func (s *Store) ListAgentRevisions(ctx context.Context, agentID uuid.UUID) ([]ConfigRevision, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, agent_id, revision, payload, created_at, COALESCE(created_by, '') FROM agent_config_revisions WHERE agent_id = $1 ORDER BY revision DESC`,
		agentID)
	if err != nil {
		return nil, fmt.Errorf("ListAgentRevisions: %w", err)
	}
	defer rows.Close()
	var out []ConfigRevision
	for rows.Next() {
		var r ConfigRevision
		var idStr, agStr string
		if err := rows.Scan(&idStr, &agStr, &r.Revision, &r.Payload, &r.CreatedAt, &r.CreatedBy); err != nil {
			return nil, err
		}
		r.ID, _ = uuid.Parse(idStr)
		r.AgentID, _ = uuid.Parse(agStr)
		out = append(out, r)
	}
	return out, rows.Err()
}

// ActivateConfigRevision sets agents.active_config_revision and invalidates cache.
func (s *Store) ActivateConfigRevision(ctx context.Context, agentID uuid.UUID, revision int) error {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM agent_config_revisions WHERE agent_id = $1 AND revision = $2`,
		agentID, revision).Scan(&n)
	if err != nil {
		return fmt.Errorf("ActivateConfigRevision: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("revision %d not found for agent", revision)
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE agents SET active_config_revision = $1, updated_at = now() WHERE id = $2`,
		revision, agentID)
	if err != nil {
		return fmt.Errorf("ActivateConfigRevision update: %w", err)
	}
	if s.rdb != nil {
		_ = s.rdb.Del(ctx, profileKeyPrefix+agentID.String()).Err()
		_ = s.rdb.Del(ctx, docsKeyPrefix+agentID.String()).Err()
	}
	return nil
}

// effectiveRevisionPayload returns payload JSON for the active revision, or latest if none active.
func (s *Store) effectiveRevisionPayload(ctx context.Context, agentID uuid.UUID) (json.RawMessage, error) {
	var active sql.NullInt32
	err := s.db.QueryRowContext(ctx, `SELECT active_config_revision FROM agents WHERE id = $1`, agentID).Scan(&active)
	if err != nil {
		return nil, err
	}
	var payload []byte
	if active.Valid && active.Int32 > 0 {
		err = s.db.QueryRowContext(ctx,
			`SELECT payload FROM agent_config_revisions WHERE agent_id = $1 AND revision = $2`,
			agentID, active.Int32).Scan(&payload)
		if err == sql.ErrNoRows {
			err = nil
		}
		if err != nil {
			return nil, err
		}
		if len(payload) > 0 {
			return payload, nil
		}
	}
	err = s.db.QueryRowContext(ctx,
		`SELECT payload FROM agent_config_revisions WHERE agent_id = $1 ORDER BY revision DESC LIMIT 1`,
		agentID).Scan(&payload)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return payload, nil
}

// mergeRevisionIntoContext overlays system_prompt from revision payload when present.
func mergeRevisionIntoContext(ac *AgentContext, payloadJSON []byte) {
	if ac == nil || len(payloadJSON) == 0 {
		return
	}
	var p ConfigRevisionPayload
	if json.Unmarshal(payloadJSON, &p) != nil {
		return
	}
	if p.SystemPrompt != "" {
		ac.SystemPrompt = p.SystemPrompt
	}
}
