package slack

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

const (
	ConfigKeySigningSecret   = "signing_secret"
	ConfigKeyClientID        = "client_id"
	ConfigKeyClientSecret    = "client_secret"
	ConfigKeyOAuthRedirectURL = "oauth_redirect_url"
)

// Store provides Slack integration persistence.
type Store struct {
	db *sql.DB
}

// NewStore returns a new Slack store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Workspace links a Slack workspace to an org.
type Workspace struct {
	ID                     uuid.UUID  `json:"id"`
	OrgID                  uuid.UUID  `json:"org_id"`
	SlackWorkspaceID       string     `json:"slack_workspace_id"`
	BotTokenRef            string     `json:"bot_token_ref,omitempty"`
	RefreshTokenRef        string     `json:"refresh_token_ref,omitempty"`
	NotificationChannelID  string     `json:"notification_channel_id,omitempty"`
	DefaultAgentID         *uuid.UUID `json:"default_agent_id,omitempty"`
	CreatedAt              string     `json:"created_at"`
	UpdatedAt              string     `json:"updated_at"`
}

// GetWorkspaceByOrgID returns the workspace linked to an org (if any).
func (s *Store) GetWorkspaceByOrgID(ctx context.Context, orgID uuid.UUID) (*Workspace, error) {
	var w Workspace
	var defaultAgentID, refreshTokenRef, notifChannel sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT id, org_id, slack_workspace_id, bot_token_ref, refresh_token_ref, notification_channel_id, default_agent_id, created_at::text, updated_at::text
		FROM slack_workspaces WHERE org_id = $1 LIMIT 1`, orgID).Scan(
		&w.ID, &w.OrgID, &w.SlackWorkspaceID, &w.BotTokenRef, &refreshTokenRef, &notifChannel, &defaultAgentID, &w.CreatedAt, &w.UpdatedAt)
	if err == nil && refreshTokenRef.Valid {
		w.RefreshTokenRef = refreshTokenRef.String
	}
	if err == nil && notifChannel.Valid {
		w.NotificationChannelID = notifChannel.String
	}
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("slack.GetWorkspaceByOrgID: %w", err)
	}
	if defaultAgentID.Valid {
		id, _ := uuid.Parse(defaultAgentID.String)
		w.DefaultAgentID = &id
	}
	return &w, nil
}

// UpdateWorkspaceDefaultAgent sets the default agent for the org's Slack workspace.
func (s *Store) UpdateWorkspaceDefaultAgent(ctx context.Context, orgID, agentID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `UPDATE slack_workspaces SET default_agent_id = $1, updated_at = now() WHERE org_id = $2`, agentID, orgID)
	return err
}

// GetWorkspaceBySlackID returns the workspace for a Slack team_id.
func (s *Store) GetWorkspaceBySlackID(ctx context.Context, slackWorkspaceID string) (*Workspace, error) {
	var w Workspace
	var defaultAgentID, refreshTokenRef, notifChannel sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT id, org_id, slack_workspace_id, bot_token_ref, refresh_token_ref, notification_channel_id, default_agent_id, created_at::text, updated_at::text
		FROM slack_workspaces WHERE slack_workspace_id = $1`, slackWorkspaceID).Scan(
		&w.ID, &w.OrgID, &w.SlackWorkspaceID, &w.BotTokenRef, &refreshTokenRef, &notifChannel, &defaultAgentID, &w.CreatedAt, &w.UpdatedAt)
	if err == nil && refreshTokenRef.Valid {
		w.RefreshTokenRef = refreshTokenRef.String
	}
	if err == nil && notifChannel.Valid {
		w.NotificationChannelID = notifChannel.String
	}
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("slack.GetWorkspaceBySlackID: %w", err)
	}
	if defaultAgentID.Valid {
		id, _ := uuid.Parse(defaultAgentID.String)
		w.DefaultAgentID = &id
	}
	return &w, nil
}

// UpsertWorkspace inserts or updates a workspace (by slack_workspace_id).
func (s *Store) UpsertWorkspace(ctx context.Context, orgID uuid.UUID, slackWorkspaceID, botTokenRef, refreshTokenRef string, defaultAgentID *uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO slack_workspaces (org_id, slack_workspace_id, bot_token_ref, refresh_token_ref, default_agent_id, updated_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (slack_workspace_id) DO UPDATE SET
			org_id = EXCLUDED.org_id,
			bot_token_ref = EXCLUDED.bot_token_ref,
			refresh_token_ref = COALESCE(NULLIF(EXCLUDED.refresh_token_ref, ''), slack_workspaces.refresh_token_ref),
			default_agent_id = EXCLUDED.default_agent_id,
			updated_at = now()`,
		orgID, slackWorkspaceID, botTokenRef, nullString(refreshTokenRef), nullUUID(defaultAgentID))
	return err
}

// UpdateWorkspaceTokens updates access and refresh tokens for a workspace (after token rotation).
func (s *Store) UpdateWorkspaceTokens(ctx context.Context, slackWorkspaceID, botTokenRef, refreshTokenRef string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE slack_workspaces SET bot_token_ref = $1, refresh_token_ref = COALESCE(NULLIF($2, ''), refresh_token_ref), updated_at = now() WHERE slack_workspace_id = $3`,
		botTokenRef, refreshTokenRef, slackWorkspaceID)
	return err
}

// UpdateWorkspaceNotificationChannel sets the default channel for proactive Slack posts (or clears it if channelID is empty).
func (s *Store) UpdateWorkspaceNotificationChannel(ctx context.Context, orgID uuid.UUID, channelID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE slack_workspaces SET notification_channel_id = $1, updated_at = now() WHERE org_id = $2`,
		nullString(channelID), orgID)
	return err
}

func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// GetChannelBinding returns agent_id for (org_id, slack_channel_id) if set.
func (s *Store) GetChannelBinding(ctx context.Context, orgID uuid.UUID, slackChannelID string) (*uuid.UUID, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT agent_id::text FROM slack_channel_bindings WHERE org_id = $1 AND slack_channel_id = $2`,
		orgID, slackChannelID).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("slack.GetChannelBinding: %w", err)
	}
	uid, _ := uuid.Parse(id)
	return &uid, nil
}

// GetUserMapping returns astra_user_id for (org_id, slack_user_id) if set.
func (s *Store) GetUserMapping(ctx context.Context, orgID uuid.UUID, slackUserID string) (*uuid.UUID, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT astra_user_id::text FROM slack_user_mappings WHERE org_id = $1 AND slack_user_id = $2`,
		orgID, slackUserID).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("slack.GetUserMapping: %w", err)
	}
	uid, _ := uuid.Parse(id)
	return &uid, nil
}

// GetConfig returns the value for a platform Slack app config key (from slack_app_config).
func (s *Store) GetConfig(ctx context.Context, key string) (string, error) {
	var val sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT value_encrypted FROM slack_app_config WHERE key = $1`, key).Scan(&val)
	if err == sql.ErrNoRows || !val.Valid {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("slack.GetConfig: %w", err)
	}
	return val.String, nil
}

// SetConfig sets a platform Slack app config key (value stored as-is; encryption can be added later).
func (s *Store) SetConfig(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO slack_app_config (key, value_encrypted, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (key) DO UPDATE SET value_encrypted = EXCLUDED.value_encrypted, updated_at = now()`,
		key, value)
	return err
}

// RootThreadTS is the value used for non-threaded (root) Slack messages.
const RootThreadTS = ""

// GetSlackSessionByThread returns chat_session_id for (workspace, channel, user, thread_ts) if exists.
func (s *Store) GetSlackSessionByThread(ctx context.Context, workspaceID, channelID, userID, threadTs string) (*uuid.UUID, error) {
	if threadTs == "" {
		threadTs = RootThreadTS
	}
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT chat_session_id::text FROM slack_sessions
		WHERE slack_workspace_id = $1 AND slack_channel_id = $2 AND slack_user_id = $3 AND slack_thread_ts = $4`,
		workspaceID, channelID, userID, threadTs).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("slack.GetSlackSessionByThread: %w", err)
	}
	uid, _ := uuid.Parse(id)
	return &uid, nil
}

// CreateSlackSession links a chat session to a Slack thread.
func (s *Store) CreateSlackSession(ctx context.Context, chatSessionID, orgID uuid.UUID, workspaceID, channelID, userID, threadTs string) error {
	if threadTs == "" {
		threadTs = RootThreadTS
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO slack_sessions (chat_session_id, org_id, slack_workspace_id, slack_channel_id, slack_user_id, slack_thread_ts)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (slack_workspace_id, slack_channel_id, slack_user_id, slack_thread_ts) DO UPDATE SET chat_session_id = EXCLUDED.chat_session_id`,
		chatSessionID, orgID, workspaceID, channelID, userID, threadTs)
	return err
}

func nullUUID(u *uuid.UUID) interface{} {
	if u == nil {
		return nil
	}
	return u.String()
}
