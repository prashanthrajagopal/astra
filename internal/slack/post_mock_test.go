package slack

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// SQL driver reuse from store_mock_test.go — slackDB/slackDriver already
// registered there. We add a separate sequence-based factory to avoid
// re-registration conflicts.
// ---------------------------------------------------------------------------

var postSeq int

func newPostDB(prepare func(string) (driver.Stmt, error)) *sql.DB {
	postSeq++
	name := fmt.Sprintf("postdb-%d", postSeq)
	sql.Register(name, &slackDriver{
		open: func(_ string) (driver.Conn, error) {
			return &slackConn{prepare: prepare}, nil
		},
	})
	db, err := sql.Open(name, "")
	if err != nil {
		panic(err)
	}
	return db
}

// workspaceRows returns rows that scan as a single Workspace row.
func workspaceRows(w *Workspace) *slackRows {
	var defaultAgentID interface{}
	if w.DefaultAgentID != nil {
		defaultAgentID = w.DefaultAgentID.String()
	}
	return &slackRows{
		cols: []string{"id", "slack_workspace_id", "bot_token_ref", "refresh_token_ref",
			"notification_channel_id", "default_agent_id", "created_at", "updated_at"},
		values: [][]driver.Value{{
			w.ID.String(),
			w.SlackWorkspaceID,
			w.BotTokenRef,
			w.RefreshTokenRef,
			w.NotificationChannelID,
			defaultAgentID,
			w.CreatedAt,
			w.UpdatedAt,
		}},
	}
}

// configRows returns rows that scan as a single config value.
func configRows(val string) *slackRows {
	return &slackRows{
		cols:   []string{"value_encrypted"},
		values: [][]driver.Value{{val}},
	}
}

// ---------------------------------------------------------------------------
// PostMessage tests
// ---------------------------------------------------------------------------

func TestPostMessage_EmptyText(t *testing.T) {
	err := PostMessage(context.Background(), nil, nil, "C1", "", "")
	if err == nil {
		t.Error("expected error for empty text")
	}
}

func TestPostMessage_NoWorkspace(t *testing.T) {
	// DB returns no rows → workspace is nil
	db := newPostDB(func(query string) (driver.Stmt, error) {
		return &slackStmt{
			numArgs: 0,
			queryFn: func(args []driver.Value) (driver.Rows, error) {
				return &slackRows{
					cols:   []string{"id", "slack_workspace_id", "bot_token_ref", "refresh_token_ref", "notification_channel_id", "default_agent_id", "created_at", "updated_at"},
					values: nil,
				}, nil
			},
		}, nil
	})
	defer db.Close()
	store := NewStore(db)

	err := PostMessage(context.Background(), store, http.DefaultClient, "C1", "hello", "")
	if err == nil {
		t.Error("expected error when no workspace")
	}
}

func TestPostMessage_GetWorkspaceDBError(t *testing.T) {
	db := newPostDB(func(query string) (driver.Stmt, error) {
		return nil, fmt.Errorf("db error")
	})
	defer db.Close()
	store := NewStore(db)

	err := PostMessage(context.Background(), store, http.DefaultClient, "C1", "hello", "")
	if err == nil {
		t.Error("expected error from DB failure")
	}
}

func TestPostMessage_NoBotToken(t *testing.T) {
	ws := &Workspace{
		ID:               uuid.New(),
		SlackWorkspaceID: "T123",
		BotTokenRef:      "", // empty
		CreatedAt:        "2024-01-01",
		UpdatedAt:        "2024-01-01",
	}
	db := newPostDB(func(query string) (driver.Stmt, error) {
		return &slackStmt{numArgs: 0, queryFn: func(args []driver.Value) (driver.Rows, error) {
			return workspaceRows(ws), nil
		}}, nil
	})
	defer db.Close()
	store := NewStore(db)

	err := PostMessage(context.Background(), store, http.DefaultClient, "C1", "hello", "")
	if err == nil {
		t.Error("expected error when bot token is empty")
	}
}

func TestPostMessage_NoChannelID(t *testing.T) {
	ws := &Workspace{
		ID:               uuid.New(),
		SlackWorkspaceID: "T123",
		BotTokenRef:      "xoxb-token",
		// NotificationChannelID empty
		CreatedAt: "2024-01-01",
		UpdatedAt: "2024-01-01",
	}
	db := newPostDB(func(query string) (driver.Stmt, error) {
		return &slackStmt{numArgs: 0, queryFn: func(args []driver.Value) (driver.Rows, error) {
			return workspaceRows(ws), nil
		}}, nil
	})
	defer db.Close()
	store := NewStore(db)

	err := PostMessage(context.Background(), store, http.DefaultClient, "", "hello", "")
	if err == nil {
		t.Error("expected error when no channel ID")
	}
}

func TestPostMessage_Success(t *testing.T) {
	// httptest server returns 200 OK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Override chatPostMessageURL via a round-tripper that redirects to test server
	client := &http.Client{
		Transport: &redirectTransport{target: srv.URL},
	}

	ws := &Workspace{
		ID:                    uuid.New(),
		SlackWorkspaceID:      "T123",
		BotTokenRef:           "xoxb-token",
		NotificationChannelID: "C456",
		CreatedAt:             "2024-01-01",
		UpdatedAt:             "2024-01-01",
	}
	db := newPostDB(func(query string) (driver.Stmt, error) {
		return &slackStmt{numArgs: 0, queryFn: func(args []driver.Value) (driver.Rows, error) {
			return workspaceRows(ws), nil
		}}, nil
	})
	defer db.Close()
	store := NewStore(db)

	err := PostMessage(context.Background(), store, client, "C456", "hello world", "")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPostMessage_WithThreadTs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["thread_ts"] == nil {
			t.Error("thread_ts should be set")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := &http.Client{Transport: &redirectTransport{target: srv.URL}}

	ws := &Workspace{
		ID: uuid.New(), SlackWorkspaceID: "T123",
		BotTokenRef: "xoxb-token", NotificationChannelID: "C456",
		CreatedAt: "2024-01-01", UpdatedAt: "2024-01-01",
	}
	db := newPostDB(func(query string) (driver.Stmt, error) {
		return &slackStmt{numArgs: 0, queryFn: func(args []driver.Value) (driver.Rows, error) {
			return workspaceRows(ws), nil
		}}, nil
	})
	defer db.Close()
	store := NewStore(db)

	err := PostMessage(context.Background(), store, client, "C456", "reply", "1234567890.123456")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPostMessage_Non200NoRefresh(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"error":"channel_not_found"}`))
	}))
	defer srv.Close()

	client := &http.Client{Transport: &redirectTransport{target: srv.URL}}

	ws := &Workspace{
		ID: uuid.New(), SlackWorkspaceID: "T123",
		BotTokenRef: "xoxb-token", NotificationChannelID: "C456",
		// No RefreshTokenRef
		CreatedAt: "2024-01-01", UpdatedAt: "2024-01-01",
	}
	db := newPostDB(func(query string) (driver.Stmt, error) {
		return &slackStmt{numArgs: 0, queryFn: func(args []driver.Value) (driver.Rows, error) {
			return workspaceRows(ws), nil
		}}, nil
	})
	defer db.Close()
	store := NewStore(db)

	err := PostMessage(context.Background(), store, client, "C456", "hello", "")
	if err == nil {
		t.Error("expected error for non-200 response")
	}
}

func TestPostMessage_UsesNotificationChannelWhenNoChannelIDProvided(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["channel"] != "C-DEFAULT" {
			t.Errorf("expected channel C-DEFAULT, got %v", body["channel"])
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := &http.Client{Transport: &redirectTransport{target: srv.URL}}

	ws := &Workspace{
		ID: uuid.New(), SlackWorkspaceID: "T123",
		BotTokenRef: "xoxb-token", NotificationChannelID: "C-DEFAULT",
		CreatedAt: "2024-01-01", UpdatedAt: "2024-01-01",
	}
	db := newPostDB(func(query string) (driver.Stmt, error) {
		return &slackStmt{numArgs: 0, queryFn: func(args []driver.Value) (driver.Rows, error) {
			return workspaceRows(ws), nil
		}}, nil
	})
	defer db.Close()
	store := NewStore(db)

	err := PostMessage(context.Background(), store, client, "", "hello", "")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// RefreshToken tests
// ---------------------------------------------------------------------------

func TestRefreshToken_MissingClientCredentials(t *testing.T) {
	// GetConfig returns empty for both client_id and client_secret
	db := newPostDB(func(query string) (driver.Stmt, error) {
		return &slackStmt{
			numArgs: 1,
			queryFn: func(args []driver.Value) (driver.Rows, error) {
				return &slackRows{cols: []string{"value_encrypted"}, values: nil}, nil
			},
		}, nil
	})
	defer db.Close()
	store := NewStore(db)

	_, _, err := RefreshToken(context.Background(), store, http.DefaultClient, "T123", "refresh-tok")
	if err == nil {
		t.Error("expected error when client credentials missing")
	}
}

func TestRefreshToken_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":            true,
			"access_token":  "new-access-token",
			"refresh_token": "new-refresh-token",
		})
	}))
	defer srv.Close()

	client := &http.Client{Transport: &redirectTransport{target: srv.URL}}

	callN := 0
	db := newPostDB(func(query string) (driver.Stmt, error) {
		return &slackStmt{
			numArgs: 1,
			queryFn: func(args []driver.Value) (driver.Rows, error) {
				callN++
				val := "client-id"
				if callN%2 == 0 {
					val = "client-secret"
				}
				return configRows(val), nil
			},
		}, nil
	})
	defer db.Close()
	store := NewStore(db)

	access, refresh, err := RefreshToken(context.Background(), store, client, "T123", "old-refresh")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if access != "new-access-token" {
		t.Errorf("access token: got %q", access)
	}
	if refresh != "new-refresh-token" {
		t.Errorf("refresh token: got %q", refresh)
	}
}

func TestRefreshToken_SlackError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":    false,
			"error": "invalid_refresh_token",
		})
	}))
	defer srv.Close()

	client := &http.Client{Transport: &redirectTransport{target: srv.URL}}

	callN := 0
	db := newPostDB(func(query string) (driver.Stmt, error) {
		return &slackStmt{
			numArgs: 1,
			queryFn: func(args []driver.Value) (driver.Rows, error) {
				callN++
				val := "client-id"
				if callN%2 == 0 {
					val = "client-secret"
				}
				return configRows(val), nil
			},
		}, nil
	})
	defer db.Close()
	store := NewStore(db)

	_, _, err := RefreshToken(context.Background(), store, client, "T123", "bad-refresh")
	if err == nil {
		t.Error("expected error for failed refresh")
	}
}

// ---------------------------------------------------------------------------
// redirectTransport rewrites all requests to the test server URL.
// ---------------------------------------------------------------------------

type redirectTransport struct {
	target string
	base   http.RoundTripper
}

func (t *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone request with target host
	clone := req.Clone(req.Context())
	clone.URL.Scheme = "http"
	clone.URL.Host = t.target[len("http://"):]
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(clone)
}

// ---------------------------------------------------------------------------
// io.EOF sentinel for slackRows when values is nil
// ---------------------------------------------------------------------------

func init() {
	// Ensure slackRows.Next returns io.EOF for nil values (already does — just verify)
	r := &slackRows{cols: []string{"x"}, values: nil}
	dest := make([]driver.Value, 1)
	if err := r.Next(dest); err != io.EOF {
		panic(fmt.Sprintf("slackRows nil-values sentinel broken: %v", err))
	}
}
