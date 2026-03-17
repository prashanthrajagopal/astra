package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// DashboardUserID is the user_id used for dashboard-initiated chat sessions (no JWT).
const DashboardUserID = "dashboard"

// Session represents a chat session.
type Session struct {
	ID        uuid.UUID  `json:"id"`
	UserID    string     `json:"user_id"`
	AgentID   uuid.UUID  `json:"agent_id"`
	Title     string     `json:"title"`
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// Message represents a chat message.
type Message struct {
	ID          uuid.UUID       `json:"id"`
	SessionID   uuid.UUID       `json:"session_id"`
	Role        string          `json:"role"`
	Content     string          `json:"content"`
	ToolCalls   json.RawMessage `json:"tool_calls,omitempty"`
	ToolResults json.RawMessage `json:"tool_results,omitempty"`
	TokensIn    int             `json:"tokens_in"`
	TokensOut   int             `json:"tokens_out"`
	CreatedAt   time.Time       `json:"created_at"`
}

// Store provides chat session and message persistence.
type Store struct {
	db *sql.DB
}

// NewStore returns a new chat store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// ListChatCapableAgents returns agents with chat_capable = true.
func (s *Store) ListChatCapableAgents(ctx context.Context) ([]struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name FROM agents WHERE chat_capable = true ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("chat.ListChatCapableAgents: %w", err)
	}
	defer rows.Close()
	var out []struct {
		ID   uuid.UUID `json:"id"`
		Name string    `json:"name"`
	}
	for rows.Next() {
		var idStr, name string
		if err := rows.Scan(&idStr, &name); err != nil {
			return nil, fmt.Errorf("chat.ListChatCapableAgents scan: %w", err)
		}
		id, _ := uuid.Parse(idStr)
		out = append(out, struct {
			ID   uuid.UUID `json:"id"`
			Name string    `json:"name"`
		}{ID: id, Name: name})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("chat.ListChatCapableAgents: %w", err)
	}
	return out, nil
}

// ListSessions returns chat sessions for the given user_id, optionally filtered by agent_id.
func (s *Store) ListSessions(ctx context.Context, userID string, agentID *uuid.UUID) ([]Session, error) {
	query := `SELECT id, user_id, agent_id, COALESCE(title,''), status, created_at, updated_at, expires_at
		FROM chat_sessions WHERE user_id = $1`
	args := []interface{}{userID}
	if agentID != nil {
		query += ` AND agent_id = $2`
		args = append(args, *agentID)
	}
	query += ` ORDER BY updated_at DESC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("chat.ListSessions: %w", err)
	}
	defer rows.Close()
	var sessions []Session
	for rows.Next() {
		var se Session
		var agentIDStr string
		var title sql.NullString
		var expiresAt sql.NullTime
		err := rows.Scan(&se.ID, &se.UserID, &agentIDStr, &title, &se.Status, &se.CreatedAt, &se.UpdatedAt, &expiresAt)
		if err != nil {
			return nil, fmt.Errorf("chat.ListSessions scan: %w", err)
		}
		se.AgentID, _ = uuid.Parse(agentIDStr)
		if title.Valid {
			se.Title = title.String
		}
		if expiresAt.Valid {
			se.ExpiresAt = &expiresAt.Time
		}
		sessions = append(sessions, se)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("chat.ListSessions: %w", err)
	}
	return sessions, nil
}

// CreateSession creates a new chat session.
func (s *Store) CreateSession(ctx context.Context, userID string, agentID uuid.UUID, title string) (*Session, error) {
	se := &Session{
		ID:      uuid.New(),
		UserID:  userID,
		AgentID: agentID,
		Title:   title,
		Status:  "active",
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO chat_sessions (id, user_id, agent_id, title, status)
		 VALUES ($1, $2, $3, $4, $5)`,
		se.ID, se.UserID, se.AgentID, se.Title, se.Status)
	if err != nil {
		return nil, fmt.Errorf("chat.CreateSession: %w", err)
	}
	_ = s.db.QueryRowContext(ctx,
		`SELECT created_at, updated_at FROM chat_sessions WHERE id = $1`,
		se.ID).Scan(&se.CreatedAt, &se.UpdatedAt)
	return se, nil
}

// GetSession returns a session by ID if it belongs to the user.
func (s *Store) GetSession(ctx context.Context, sessionID uuid.UUID, userID string) (*Session, error) {
	var se Session
	var agentIDStr string
	var title sql.NullString
	var expiresAt sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, agent_id, COALESCE(title,''), status, created_at, updated_at, expires_at
		 FROM chat_sessions WHERE id = $1 AND user_id = $2`,
		sessionID, userID).Scan(&se.ID, &se.UserID, &agentIDStr, &title, &se.Status, &se.CreatedAt, &se.UpdatedAt, &expiresAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("chat.GetSession: %w", err)
	}
	se.AgentID, _ = uuid.Parse(agentIDStr)
	if title.Valid {
		se.Title = title.String
	}
	if expiresAt.Valid {
		se.ExpiresAt = &expiresAt.Time
	}
	return &se, nil
}

// GetMessages returns messages for a session, most recent last.
func (s *Store) GetMessages(ctx context.Context, sessionID uuid.UUID, userID string) ([]Message, error) {
	var exists int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM chat_sessions WHERE id = $1 AND user_id = $2`,
		sessionID, userID).Scan(&exists)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("chat.GetMessages session check: %w", err)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, role, content, tool_calls, tool_results, COALESCE(tokens_in,0), COALESCE(tokens_out,0), created_at
		 FROM chat_messages WHERE session_id = $1 ORDER BY created_at ASC`,
		sessionID)
	if err != nil {
		return nil, fmt.Errorf("chat.GetMessages: %w", err)
	}
	defer rows.Close()
	var msgs []Message
	for rows.Next() {
		var m Message
		var sessionIDStr string
		var toolCalls, toolResults sql.NullString
		err := rows.Scan(&m.ID, &sessionIDStr, &m.Role, &m.Content, &toolCalls, &toolResults, &m.TokensIn, &m.TokensOut, &m.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("chat.GetMessages scan: %w", err)
		}
		m.SessionID, _ = uuid.Parse(sessionIDStr)
		if toolCalls.Valid && toolCalls.String != "" {
			m.ToolCalls = []byte(toolCalls.String)
		}
		if toolResults.Valid && toolResults.String != "" {
			m.ToolResults = []byte(toolResults.String)
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("chat.GetMessages: %w", err)
	}
	return msgs, nil
}

// AppendMessage inserts a message into a session. Verifies session belongs to user.
func (s *Store) AppendMessage(ctx context.Context, sessionID uuid.UUID, userID string, role, content string, toolCalls, toolResults json.RawMessage, tokensIn, tokensOut int) (*Message, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM chat_sessions WHERE id = $1 AND user_id = $2`,
		sessionID, userID).Scan(&exists); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("chat.AppendMessage session check: %w", err)
	}

	m := &Message{
		ID:        uuid.New(),
		SessionID: sessionID,
		Role:      role,
		Content:   content,
		TokensIn:  tokensIn,
		TokensOut: tokensOut,
	}
	toolCallsVal := interface{}(nil)
	if len(toolCalls) > 0 {
		toolCallsVal = toolCalls
	}
	toolResultsVal := interface{}(nil)
	if len(toolResults) > 0 {
		toolResultsVal = toolResults
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO chat_messages (id, session_id, role, content, tool_calls, tool_results, tokens_in, tokens_out)
		 VALUES ($1, $2, $3, $4, $5::jsonb, $6::jsonb, $7, $8)`,
		m.ID, m.SessionID, m.Role, m.Content, toolCallsVal, toolResultsVal, m.TokensIn, m.TokensOut)
	if err != nil {
		return nil, fmt.Errorf("chat.AppendMessage: %w", err)
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE chat_sessions SET updated_at = now() WHERE id = $1`, sessionID)
	_ = s.db.QueryRowContext(ctx, `SELECT created_at FROM chat_messages WHERE id = $1`, m.ID).Scan(&m.CreatedAt)
	return m, nil
}

// GetSessionTokenTotal returns the sum of tokens_in + tokens_out for all messages in the session.
func (s *Store) GetSessionTokenTotal(ctx context.Context, sessionID uuid.UUID) (int, error) {
	var total int
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(COALESCE(tokens_in, 0) + COALESCE(tokens_out, 0)), 0) FROM chat_messages WHERE session_id = $1`,
		sessionID).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("chat.GetSessionTokenTotal: %w", err)
	}
	return total, nil
}
