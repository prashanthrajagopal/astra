package slack

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

// TestGetDefaultWorkspace_WithDefaultAgentID covers the defaultAgentID.Valid branch.
func TestGetDefaultWorkspace_WithDefaultAgentID(t *testing.T) {
	agentID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	db := newSlackDB(func(query string) (driver.Stmt, error) {
		return &slackStmt{
			numArgs: 0,
			queryFn: func(args []driver.Value) (driver.Rows, error) {
				return &slackRows{
					cols: []string{"id", "slack_workspace_id", "bot_token_ref",
						"refresh_token_ref", "notification_channel_id",
						"default_agent_id", "created_at", "updated_at"},
					values: [][]driver.Value{{
						uuid.New().String(),
						"T999",
						"xoxb-bot",
						"xoxe-refresh",
						"C-notif",
						agentID.String(), // valid agent ID
						"2024-01-01",
						"2024-06-01",
					}},
				}, nil
			},
		}, nil
	})
	defer db.Close()

	s := NewStore(db)
	w, err := s.GetDefaultWorkspace(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w == nil {
		t.Fatal("expected non-nil workspace")
	}
	if w.DefaultAgentID == nil {
		t.Fatal("expected non-nil DefaultAgentID")
	}
	if *w.DefaultAgentID != agentID {
		t.Errorf("DefaultAgentID: got %v, want %v", *w.DefaultAgentID, agentID)
	}
	if w.RefreshTokenRef != "xoxe-refresh" {
		t.Errorf("RefreshTokenRef: got %q", w.RefreshTokenRef)
	}
	if w.NotificationChannelID != "C-notif" {
		t.Errorf("NotificationChannelID: got %q", w.NotificationChannelID)
	}
}

// TestGetWorkspaceBySlackID_Found covers the success + defaultAgentID.Valid branch.
func TestGetWorkspaceBySlackID_Found(t *testing.T) {
	agentID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	db := newSlackDB(func(query string) (driver.Stmt, error) {
		return &slackStmt{
			numArgs: 1,
			queryFn: func(args []driver.Value) (driver.Rows, error) {
				return &slackRows{
					cols: []string{"id", "slack_workspace_id", "bot_token_ref",
						"refresh_token_ref", "notification_channel_id",
						"default_agent_id", "created_at", "updated_at"},
					values: [][]driver.Value{{
						uuid.New().String(),
						"T-FOUND",
						"xoxb-found",
						"",    // refresh empty — NullString not valid
						"",    // notif channel empty
						agentID.String(),
						"2024-01-01",
						"2024-06-01",
					}},
				}, nil
			},
		}, nil
	})
	defer db.Close()

	s := NewStore(db)
	w, err := s.GetWorkspaceBySlackID(context.Background(), "T-FOUND")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w == nil {
		t.Fatal("expected non-nil workspace")
	}
	if w.SlackWorkspaceID != "T-FOUND" {
		t.Errorf("SlackWorkspaceID: got %q", w.SlackWorkspaceID)
	}
	if w.DefaultAgentID == nil || *w.DefaultAgentID != agentID {
		t.Errorf("DefaultAgentID: got %v, want %v", w.DefaultAgentID, agentID)
	}
}

// TestGetSlackSessionByThread_Found covers the success path.
func TestGetSlackSessionByThread_Found(t *testing.T) {
	sessionID := uuid.MustParse("cafecafe-cafe-cafe-cafe-cafecafecafe")
	db := newSlackDB(func(query string) (driver.Stmt, error) {
		return &slackStmt{
			numArgs: 4,
			queryFn: func(args []driver.Value) (driver.Rows, error) {
				return &slackRows{
					cols:   []string{"chat_session_id"},
					values: [][]driver.Value{{sessionID.String()}},
				}, nil
			},
		}, nil
	})
	defer db.Close()

	s := NewStore(db)
	got, err := s.GetSlackSessionByThread(context.Background(), "WS1", "C1", "U1", "ts.001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || *got != sessionID {
		t.Errorf("got %v, want %v", got, sessionID)
	}
}

// TestPostMessage_TokenRefreshSucceeds covers the newAccess != "" branch in PostMessage.
func TestPostMessage_TokenRefreshSucceeds(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.URL.Path == "/api/chat.postMessage" {
			if callCount == 1 {
				// First call: return 401 to trigger refresh
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"ok":false,"error":"token_expired"}`))
			} else {
				// After refresh: success
				w.WriteHeader(http.StatusOK)
			}
		} else if r.URL.Path == "/api/oauth.v2.access" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":            true,
				"access_token":  "new-access-token",
				"refresh_token": "new-refresh-token",
			})
		}
	}))
	defer srv.Close()

	client := &http.Client{Transport: &redirectTransport{target: srv.URL}}

	ws := &Workspace{
		ID:                    uuid.New(),
		SlackWorkspaceID:      "T123",
		BotTokenRef:           "old-token",
		RefreshTokenRef:       "old-refresh",
		NotificationChannelID: "C456",
		CreatedAt:             "2024-01-01",
		UpdatedAt:             "2024-01-01",
	}

	dbCallN := 0
	db := newPostDB(func(query string) (driver.Stmt, error) {
		dbCallN++
		switch {
		case dbCallN == 1:
			// GetDefaultWorkspace
			return &slackStmt{numArgs: 0, queryFn: func(args []driver.Value) (driver.Rows, error) {
				return workspaceRows(ws), nil
			}}, nil
		case dbCallN == 2:
			// GetConfig(client_id)
			return &slackStmt{numArgs: 1, queryFn: func(args []driver.Value) (driver.Rows, error) {
				return configRows("my-client-id"), nil
			}}, nil
		case dbCallN == 3:
			// GetConfig(client_secret)
			return &slackStmt{numArgs: 1, queryFn: func(args []driver.Value) (driver.Rows, error) {
				return configRows("my-client-secret"), nil
			}}, nil
		default:
			// UpdateWorkspaceTokens
			return &slackStmt{numArgs: 3, execFn: func(args []driver.Value) (driver.Result, error) {
				return &slackResult{}, nil
			}}, nil
		}
	})
	defer db.Close()
	store := NewStore(db)

	err := PostMessage(context.Background(), store, client, "C456", "hello", "")
	if err != nil {
		t.Errorf("unexpected error after token refresh: %v", err)
	}
}

// TestPostMessage_TokenRefreshFails covers the refresh error path.
func TestPostMessage_TokenRefreshFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/chat.postMessage" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"ok":false,"error":"token_expired"}`))
		} else if r.URL.Path == "/api/oauth.v2.access" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":    false,
				"error": "invalid_refresh_token",
			})
		}
	}))
	defer srv.Close()

	client := &http.Client{Transport: &redirectTransport{target: srv.URL}}

	ws := &Workspace{
		ID: uuid.New(), SlackWorkspaceID: "T123",
		BotTokenRef: "old-token", RefreshTokenRef: "bad-refresh",
		NotificationChannelID: "C456",
		CreatedAt: "2024-01-01", UpdatedAt: "2024-01-01",
	}

	dbCallN := 0
	db := newPostDB(func(query string) (driver.Stmt, error) {
		dbCallN++
		switch dbCallN {
		case 1:
			return &slackStmt{numArgs: 0, queryFn: func(args []driver.Value) (driver.Rows, error) {
				return workspaceRows(ws), nil
			}}, nil
		case 2:
			return &slackStmt{numArgs: 1, queryFn: func(args []driver.Value) (driver.Rows, error) {
				return configRows("cid"), nil
			}}, nil
		default:
			return &slackStmt{numArgs: 1, queryFn: func(args []driver.Value) (driver.Rows, error) {
				return configRows("csec"), nil
			}}, nil
		}
	})
	defer db.Close()
	store := NewStore(db)

	err := PostMessage(context.Background(), store, client, "C456", "hello", "")
	if err == nil {
		t.Error("expected error when token refresh fails")
	}
}

// TestGetConfig_Found covers the success path returning a valid value.
func TestGetConfig_Found(t *testing.T) {
	db := newSlackDB(func(query string) (driver.Stmt, error) {
		return &slackStmt{
			numArgs: 1,
			queryFn: func(args []driver.Value) (driver.Rows, error) {
				return configRows("my-secret-value"), nil
			},
		}, nil
	})
	defer db.Close()

	s := NewStore(db)
	val, err := s.GetConfig(context.Background(), "signing_secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "my-secret-value" {
		t.Errorf("got %q, want my-secret-value", val)
	}
}

// TestRefreshToken_HTTPError covers network failure in RefreshToken.
func TestRefreshToken_HTTPError(t *testing.T) {
	// Server that immediately closes connection
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// hijack not available in httptest simply, return invalid JSON
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "not-json{{{")
	}))
	defer srv.Close()

	client := &http.Client{Transport: &redirectTransport{target: srv.URL}}

	dbCallN := 0
	db := newPostDB(func(query string) (driver.Stmt, error) {
		dbCallN++
		if dbCallN == 1 {
			return &slackStmt{numArgs: 1, queryFn: func(args []driver.Value) (driver.Rows, error) {
				return configRows("cid"), nil
			}}, nil
		}
		return &slackStmt{numArgs: 1, queryFn: func(args []driver.Value) (driver.Rows, error) {
			return configRows("csec"), nil
		}}, nil
	})
	defer db.Close()
	store := NewStore(db)

	_, _, err := RefreshToken(context.Background(), store, client, "T123", "refresh")
	if err == nil {
		t.Error("expected error for invalid JSON response")
	}
}
