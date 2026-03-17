package agentdocs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	profileKeyPrefix     = "agent:profile:"
	docsKeyPrefix        = "agent:docs:"
	chatCapableKeyPrefix = "agent:chat_capable:"
	agentPromptKeyPrefix = "agent:prompt:"
)

type DocType string

const (
	DocTypeRule       DocType = "rule"
	DocTypeSkill      DocType = "skill"
	DocTypeContextDoc DocType = "context_doc"
	DocTypeReference  DocType = "reference"
)

type Document struct {
	ID        uuid.UUID       `json:"id"`
	AgentID   uuid.UUID       `json:"agent_id"`
	GoalID    *uuid.UUID      `json:"goal_id,omitempty"`
	DocType   DocType         `json:"doc_type"`
	Name      string          `json:"name"`
	Content   *string         `json:"content,omitempty"`
	URI       *string         `json:"uri,omitempty"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	Priority  int             `json:"priority"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type AgentProfile struct {
	ID                        uuid.UUID       `json:"id"`
	Name                      string          `json:"name"`
	ActorType                 string          `json:"actor_type,omitempty"`
	SystemPrompt              string          `json:"system_prompt"`
	Config                    json.RawMessage `json:"config"`
	ChatCapable               bool            `json:"chat_capable,omitempty"`
	IngestSourceType          string          `json:"ingest_source_type,omitempty"`
	IngestSourceConfig        json.RawMessage `json:"ingest_source_config,omitempty"`
	SlackNotificationsEnabled bool            `json:"slack_notifications_enabled,omitempty"`
}

// IngestBinding is returned by GetIngestBindings for adapters to know which agent listens to which source.
type IngestBinding struct {
	AgentID            uuid.UUID       `json:"agent_id"`
	IngestSourceType   string          `json:"ingest_source_type"`
	IngestSourceConfig json.RawMessage `json:"ingest_source_config"`
}

type ListOptions struct {
	DocType    *DocType
	GoalID     *uuid.UUID
	GlobalOnly bool
}

type Store struct {
	db  *sql.DB
	rdb *redis.Client
	ttl time.Duration
}

func NewStore(db *sql.DB, rdb *redis.Client) *Store {
	return &Store{db: db, rdb: rdb, ttl: 5 * time.Minute}
}

func (s *Store) GetProfile(ctx context.Context, agentID uuid.UUID) (*AgentProfile, error) {
	if s.rdb != nil {
		key := profileKeyPrefix + agentID.String()
		data, err := s.rdb.Get(ctx, key).Bytes()
		if err == nil {
			var p AgentProfile
			if err := json.Unmarshal(data, &p); err != nil {
				return nil, fmt.Errorf("agentdocs.GetProfile unmarshal: %w", err)
			}
			return &p, nil
		}
		if err != redis.Nil {
			return nil, fmt.Errorf("agentdocs.GetProfile redis: %w", err)
		}
	}

	var p AgentProfile
	var idStr, name, actorType sql.NullString
	var systemPrompt sql.NullString
	var config []byte
	var ingestType sql.NullString
	var ingestConfig []byte
	var chatCapable sql.NullBool
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, COALESCE(actor_type, ''), system_prompt, config, chat_capable, ingest_source_type, ingest_source_config, slack_notifications_enabled
		 FROM agents WHERE id = $1`,
		agentID).Scan(&idStr, &name, &actorType, &systemPrompt, &config, &chatCapable, &ingestType, &ingestConfig, &p.SlackNotificationsEnabled)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("agentdocs.GetProfile: %w", err)
	}
	p.ID = agentID
	if name.Valid {
		p.Name = name.String
	}
	if actorType.Valid {
		p.ActorType = actorType.String
	}
	if systemPrompt.Valid {
		p.SystemPrompt = systemPrompt.String
	}
	if len(config) > 0 {
		p.Config = config
	}
	if chatCapable.Valid {
		p.ChatCapable = chatCapable.Bool
	}
	if ingestType.Valid {
		p.IngestSourceType = ingestType.String
	}
	if len(ingestConfig) > 0 {
		p.IngestSourceConfig = ingestConfig
	}
	if s.rdb != nil {
		data, _ := json.Marshal(&p)
		_ = s.rdb.SetEx(ctx, profileKeyPrefix+agentID.String(), data, s.ttl).Err()
	}
	return &p, nil
}

func (s *Store) UpdateProfile(ctx context.Context, agentID uuid.UUID, systemPrompt *string, config *json.RawMessage) error {
	if systemPrompt == nil && config == nil {
		return nil
	}
	updates := []string{}
	args := []interface{}{agentID}
	argIdx := 2
	if systemPrompt != nil {
		updates = append(updates, fmt.Sprintf("system_prompt = $%d", argIdx))
		args = append(args, *systemPrompt)
		argIdx++
	}
	if config != nil {
		updates = append(updates, fmt.Sprintf("config = $%d::jsonb", argIdx))
		args = append(args, config)
	}
	if len(updates) == 0 {
		return nil
	}
	q := fmt.Sprintf("UPDATE agents SET %s, updated_at = now() WHERE id = $1", joinStrings(updates, ", "))
	_, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("agentdocs.UpdateProfile: %w", err)
	}
	if s.rdb != nil {
		_ = s.rdb.Del(ctx, profileKeyPrefix+agentID.String()).Err()
		_ = s.rdb.Del(ctx, docsKeyPrefix+agentID.String()).Err()
	}
	return nil
}

// GetIngestBindings returns all agents that have an ingest source configured (for adapters to subscribe and forward).
func (s *Store) GetIngestBindings(ctx context.Context) ([]IngestBinding, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, ingest_source_type, ingest_source_config FROM agents WHERE ingest_source_type IS NOT NULL AND ingest_source_type != ''`)
	if err != nil {
		return nil, fmt.Errorf("agentdocs.GetIngestBindings: %w", err)
	}
	defer rows.Close()
	var out []IngestBinding
	for rows.Next() {
		var b IngestBinding
		var idStr string
		var cfg []byte
		if err := rows.Scan(&idStr, &b.IngestSourceType, &cfg); err != nil {
			return nil, fmt.Errorf("agentdocs.GetIngestBindings scan: %w", err)
		}
		b.AgentID, _ = uuid.Parse(idStr)
		if len(cfg) > 0 {
			b.IngestSourceConfig = cfg
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// UpdateAgentIngestSource sets or clears the external data source for an agent. Empty sourceType clears it.
func (s *Store) UpdateAgentIngestSource(ctx context.Context, agentID uuid.UUID, sourceType string, sourceConfig json.RawMessage) error {
	if sourceType == "" {
		_, err := s.db.ExecContext(ctx, `UPDATE agents SET ingest_source_type = NULL, ingest_source_config = NULL, updated_at = now() WHERE id = $1`, agentID)
		if err != nil {
			return fmt.Errorf("agentdocs.UpdateAgentIngestSource: %w", err)
		}
	} else {
		_, err := s.db.ExecContext(ctx, `UPDATE agents SET ingest_source_type = $1, ingest_source_config = $2, updated_at = now() WHERE id = $3`,
			sourceType, nullJSON(sourceConfig), agentID)
		if err != nil {
			return fmt.Errorf("agentdocs.UpdateAgentIngestSource: %w", err)
		}
	}
	if s.rdb != nil {
		_ = s.rdb.Del(ctx, profileKeyPrefix+agentID.String()).Err()
	}
	return nil
}

// UpdateAgentSlackNotifications sets whether the agent may post to Slack (e.g. when prompt instructs).
func (s *Store) UpdateAgentSlackNotifications(ctx context.Context, agentID uuid.UUID, enabled bool) error {
	_, err := s.db.ExecContext(ctx, `UPDATE agents SET slack_notifications_enabled = $1, updated_at = now() WHERE id = $2`, enabled, agentID)
	if err != nil {
		return fmt.Errorf("agentdocs.UpdateAgentSlackNotifications: %w", err)
	}
	if s.rdb != nil {
		_ = s.rdb.Del(ctx, profileKeyPrefix+agentID.String()).Err()
	}
	return nil
}

func nullJSON(b json.RawMessage) interface{} {
	if len(b) == 0 {
		return nil
	}
	return b
}

// UpdateAgentName sets the agent's display name and invalidates profile cache.
func (s *Store) UpdateAgentName(ctx context.Context, agentID uuid.UUID, name string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE agents SET name = $1, updated_at = now() WHERE id = $2`, name, agentID)
	if err != nil {
		return fmt.Errorf("agentdocs.UpdateAgentName: %w", err)
	}
	if s.rdb != nil {
		_ = s.rdb.Del(ctx, profileKeyPrefix+agentID.String()).Err()
	}
	return nil
}

// UpdateAgentMeta updates chat_capable on an agent (single-platform; visibility/org columns removed).
func (s *Store) UpdateAgentMeta(ctx context.Context, agentID uuid.UUID, _ string, chatCapable bool, _, _, _ *uuid.UUID) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE agents SET chat_capable = $1, updated_at = now() WHERE id = $2`,
		chatCapable, agentID)
	if err != nil {
		return fmt.Errorf("agentdocs.UpdateAgentMeta: %w", err)
	}
	if s.rdb != nil {
		_ = s.rdb.Del(ctx, profileKeyPrefix+agentID.String()).Err()
		_ = s.rdb.Del(ctx, docsKeyPrefix+agentID.String()).Err()
	}
	return nil
}

// SetAgentPromptCache stores the system prompt in Redis for fast access (24h TTL).
func (s *Store) SetAgentPromptCache(ctx context.Context, agentID uuid.UUID, prompt string) {
	if s.rdb != nil && prompt != "" {
		_ = s.rdb.Set(ctx, agentPromptKeyPrefix+agentID.String(), prompt, 24*time.Hour).Err()
	}
}

// UpdateAgentStatus sets the agent's status (e.g. "active", "stopped"). Invalid status is rejected by DB constraint.
func (s *Store) UpdateAgentStatus(ctx context.Context, agentID uuid.UUID, status string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE agents SET status = $1, updated_at = now() WHERE id = $2`, status, agentID)
	if err != nil {
		return fmt.Errorf("agentdocs.UpdateAgentStatus: %w", err)
	}
	if s.rdb != nil {
		_ = s.rdb.Del(ctx, profileKeyPrefix+agentID.String()).Err()
		_ = s.rdb.Del(ctx, docsKeyPrefix+agentID.String()).Err()
	}
	return nil
}

// DeleteAgent removes the agent and its dependent data in order: task_dependencies, tasks for agent's goals, then agent (cascades to goals, agent_documents, memories, phase_runs).
func (s *Store) DeleteAgent(ctx context.Context, agentID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM task_dependencies WHERE task_id IN (SELECT id FROM tasks WHERE goal_id IN (SELECT id FROM goals WHERE agent_id = $1));
		DELETE FROM tasks WHERE goal_id IN (SELECT id FROM goals WHERE agent_id = $1);
		DELETE FROM agents WHERE id = $1;
	`, agentID)
	if err != nil {
		return fmt.Errorf("agentdocs.DeleteAgent: %w", err)
	}
	if s.rdb != nil {
		_ = s.rdb.Del(ctx, profileKeyPrefix+agentID.String()).Err()
		_ = s.rdb.Del(ctx, docsKeyPrefix+agentID.String()).Err()
	}
	return nil
}

func joinStrings(ss []string, sep string) string {
	if len(ss) == 0 {
		return ""
	}
	r := ss[0]
	for i := 1; i < len(ss); i++ {
		r += sep + ss[i]
	}
	return r
}

func (s *Store) CreateDocument(ctx context.Context, doc *Document) error {
	if doc.ID == uuid.Nil {
		doc.ID = uuid.New()
	}
	contentVal := interface{}(nil)
	if doc.Content != nil {
		contentVal = *doc.Content
	}
	uriVal := interface{}(nil)
	if doc.URI != nil {
		uriVal = *doc.URI
	}
	goalVal := interface{}(nil)
	if doc.GoalID != nil {
		goalVal = doc.GoalID
	}
	metadata := doc.Metadata
	if metadata == nil {
		metadata = []byte("{}")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_documents (id, agent_id, goal_id, doc_type, name, content, uri, metadata, priority)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9)`,
		doc.ID, doc.AgentID, goalVal, string(doc.DocType), doc.Name, contentVal, uriVal, metadata, doc.Priority)
	if err != nil {
		return fmt.Errorf("agentdocs.CreateDocument: %w", err)
	}
	if s.rdb != nil {
		_ = s.rdb.Del(ctx, docsKeyPrefix+doc.AgentID.String()).Err()
	}
	return nil
}

func (s *Store) ListDocuments(ctx context.Context, agentID uuid.UUID, opts ListOptions) ([]Document, error) {
	hasFilters := opts.DocType != nil || opts.GoalID != nil || opts.GlobalOnly

	if !hasFilters && s.rdb != nil {
		key := docsKeyPrefix + agentID.String()
		data, err := s.rdb.Get(ctx, key).Bytes()
		if err == nil {
			var docs []Document
			if err := json.Unmarshal(data, &docs); err != nil {
				return nil, fmt.Errorf("agentdocs.ListDocuments unmarshal: %w", err)
			}
			return docs, nil
		}
		if err != redis.Nil {
			return nil, fmt.Errorf("agentdocs.ListDocuments redis: %w", err)
		}
	}

	query := `SELECT id, agent_id, goal_id, doc_type, name, content, uri, metadata, priority, created_at, updated_at
		FROM agent_documents WHERE agent_id = $1`
	args := []interface{}{agentID}
	argIdx := 2
	if opts.GlobalOnly {
		query += " AND goal_id IS NULL"
	}
	if opts.DocType != nil {
		query += fmt.Sprintf(" AND doc_type = $%d", argIdx)
		args = append(args, string(*opts.DocType))
		argIdx++
	}
	if opts.GoalID != nil {
		query += fmt.Sprintf(" AND goal_id = $%d", argIdx)
		args = append(args, *opts.GoalID)
	}
	query += " ORDER BY priority ASC, created_at ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("agentdocs.ListDocuments: %w", err)
	}
	defer rows.Close()

	var docs []Document
	for rows.Next() {
		var d Document
		var goalID sql.NullString
		var idStr, agentIDStr string
		var docType string
		var content, uri sql.NullString
		var metadata []byte
		err := rows.Scan(&idStr, &agentIDStr, &goalID, &docType, &d.Name, &content, &uri, &metadata, &d.Priority, &d.CreatedAt, &d.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("agentdocs.ListDocuments scan: %w", err)
		}
		d.ID, _ = uuid.Parse(idStr)
		d.AgentID, _ = uuid.Parse(agentIDStr)
		if goalID.Valid {
			g, _ := uuid.Parse(goalID.String)
			d.GoalID = &g
		}
		d.DocType = DocType(docType)
		if content.Valid {
			d.Content = &content.String
		}
		if uri.Valid {
			d.URI = &uri.String
		}
		if len(metadata) > 0 {
			d.Metadata = metadata
		}
		docs = append(docs, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("agentdocs.ListDocuments: %w", err)
	}

	if !hasFilters && s.rdb != nil {
		data, _ := json.Marshal(docs)
		_ = s.rdb.SetEx(ctx, docsKeyPrefix+agentID.String(), data, s.ttl).Err()
	}

	return docs, nil
}

func (s *Store) DeleteDocument(ctx context.Context, docID uuid.UUID) error {
	var agentIDStr string
	err := s.db.QueryRowContext(ctx, `SELECT agent_id FROM agent_documents WHERE id = $1`, docID).Scan(&agentIDStr)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("agentdocs.DeleteDocument lookup: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM agent_documents WHERE id = $1`, docID)
	if err != nil {
		return fmt.Errorf("agentdocs.DeleteDocument: %w", err)
	}
	if s.rdb != nil {
		_ = s.rdb.Del(ctx, docsKeyPrefix+agentIDStr).Err()
	}
	return nil
}
