package slack

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"testing"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Minimal in-process SQL driver for slack tests
// ---------------------------------------------------------------------------

type slackDriver struct {
	open func(string) (driver.Conn, error)
}

func (d *slackDriver) Open(name string) (driver.Conn, error) { return d.open(name) }

type slackConn struct {
	prepare func(string) (driver.Stmt, error)
}

func (c *slackConn) Prepare(query string) (driver.Stmt, error) { return c.prepare(query) }
func (c *slackConn) Close() error                              { return nil }
func (c *slackConn) Begin() (driver.Tx, error)                 { return nil, fmt.Errorf("no tx") }

type slackStmt struct {
	queryFn func([]driver.Value) (driver.Rows, error)
	execFn  func([]driver.Value) (driver.Result, error)
	numArgs int
}

func (s *slackStmt) Close() error { return nil }
func (s *slackStmt) NumInput() int { return s.numArgs }
func (s *slackStmt) Exec(args []driver.Value) (driver.Result, error) {
	if s.execFn != nil {
		return s.execFn(args)
	}
	return &slackResult{}, nil
}
func (s *slackStmt) Query(args []driver.Value) (driver.Rows, error) {
	if s.queryFn != nil {
		return s.queryFn(args)
	}
	return &slackRows{cols: []string{}}, nil
}

type slackResult struct{}

func (r *slackResult) LastInsertId() (int64, error) { return 0, nil }
func (r *slackResult) RowsAffected() (int64, error) { return 1, nil }

type slackRows struct {
	cols    []string
	values  [][]driver.Value
	current int
}

func (r *slackRows) Columns() []string { return r.cols }
func (r *slackRows) Close() error      { return nil }
func (r *slackRows) Next(dest []driver.Value) error {
	if r.current >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.current])
	r.current++
	return nil
}

var slackSeq int

func newSlackDB(prepare func(string) (driver.Stmt, error)) *sql.DB {
	slackSeq++
	name := fmt.Sprintf("slackdb-%d", slackSeq)
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

// ---------------------------------------------------------------------------
// NewStore
// ---------------------------------------------------------------------------

func TestNewStore(t *testing.T) {
	db := newSlackDB(func(string) (driver.Stmt, error) { return nil, fmt.Errorf("unused") })
	defer db.Close()
	s := NewStore(db)
	if s == nil {
		t.Fatal("expected non-nil store")
	}
}

// ---------------------------------------------------------------------------
// GetDefaultWorkspace
// ---------------------------------------------------------------------------

func TestGetDefaultWorkspace_NoRows(t *testing.T) {
	db := newSlackDB(func(query string) (driver.Stmt, error) {
		return &slackStmt{
			numArgs: 0,
			queryFn: func(args []driver.Value) (driver.Rows, error) {
				return &slackRows{
					cols:   []string{"id", "slack_workspace_id", "bot_token_ref", "refresh_token_ref", "notification_channel_id", "default_agent_id", "created_at", "updated_at"},
					values: nil, // no rows
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
	if w != nil {
		t.Errorf("expected nil workspace for no rows, got %+v", w)
	}
}

func TestGetDefaultWorkspace_DBError(t *testing.T) {
	db := newSlackDB(func(query string) (driver.Stmt, error) {
		return nil, fmt.Errorf("connection error")
	})
	defer db.Close()

	s := NewStore(db)
	_, err := s.GetDefaultWorkspace(context.Background())
	if err == nil {
		t.Error("expected error")
	}
}

// ---------------------------------------------------------------------------
// GetWorkspaceBySlackID
// ---------------------------------------------------------------------------

func TestGetWorkspaceBySlackID_NoRows(t *testing.T) {
	db := newSlackDB(func(query string) (driver.Stmt, error) {
		return &slackStmt{
			numArgs: 1,
			queryFn: func(args []driver.Value) (driver.Rows, error) {
				return &slackRows{
					cols:   []string{"id", "slack_workspace_id", "bot_token_ref", "refresh_token_ref", "notification_channel_id", "default_agent_id", "created_at", "updated_at"},
					values: nil,
				}, nil
			},
		}, nil
	})
	defer db.Close()

	s := NewStore(db)
	w, err := s.GetWorkspaceBySlackID(context.Background(), "T-NOTFOUND")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w != nil {
		t.Errorf("expected nil workspace, got %+v", w)
	}
}

func TestGetWorkspaceBySlackID_DBError(t *testing.T) {
	db := newSlackDB(func(query string) (driver.Stmt, error) {
		return nil, fmt.Errorf("db error")
	})
	defer db.Close()

	s := NewStore(db)
	_, err := s.GetWorkspaceBySlackID(context.Background(), "T123")
	if err == nil {
		t.Error("expected error")
	}
}

// ---------------------------------------------------------------------------
// UpsertWorkspace
// ---------------------------------------------------------------------------

func TestUpsertWorkspace_Success(t *testing.T) {
	db := newSlackDB(func(query string) (driver.Stmt, error) {
		return &slackStmt{
			numArgs: 4,
			execFn: func(args []driver.Value) (driver.Result, error) {
				return &slackResult{}, nil
			},
		}, nil
	})
	defer db.Close()

	s := NewStore(db)
	agentID := uuid.New()
	err := s.UpsertWorkspace(context.Background(), "T123", "bot-ref", "refresh-ref", &agentID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUpsertWorkspace_NilAgentID(t *testing.T) {
	db := newSlackDB(func(query string) (driver.Stmt, error) {
		return &slackStmt{
			numArgs: 4,
			execFn: func(args []driver.Value) (driver.Result, error) {
				return &slackResult{}, nil
			},
		}, nil
	})
	defer db.Close()

	s := NewStore(db)
	err := s.UpsertWorkspace(context.Background(), "T123", "bot-ref", "", nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUpsertWorkspace_DBError(t *testing.T) {
	db := newSlackDB(func(query string) (driver.Stmt, error) {
		return nil, fmt.Errorf("exec error")
	})
	defer db.Close()

	s := NewStore(db)
	err := s.UpsertWorkspace(context.Background(), "T123", "ref", "", nil)
	if err == nil {
		t.Error("expected error")
	}
}

// ---------------------------------------------------------------------------
// UpdateWorkspaceTokens
// ---------------------------------------------------------------------------

func TestUpdateWorkspaceTokens_Success(t *testing.T) {
	db := newSlackDB(func(query string) (driver.Stmt, error) {
		return &slackStmt{numArgs: 3, execFn: func(args []driver.Value) (driver.Result, error) {
			return &slackResult{}, nil
		}}, nil
	})
	defer db.Close()

	s := NewStore(db)
	err := s.UpdateWorkspaceTokens(context.Background(), "T123", "new-bot", "new-refresh")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUpdateWorkspaceTokens_DBError(t *testing.T) {
	db := newSlackDB(func(query string) (driver.Stmt, error) {
		return nil, fmt.Errorf("update error")
	})
	defer db.Close()

	s := NewStore(db)
	err := s.UpdateWorkspaceTokens(context.Background(), "T123", "bot", "refresh")
	if err == nil {
		t.Error("expected error")
	}
}

// ---------------------------------------------------------------------------
// UpdateWorkspaceNotificationChannel
// ---------------------------------------------------------------------------

func TestUpdateWorkspaceNotificationChannel_Success(t *testing.T) {
	db := newSlackDB(func(query string) (driver.Stmt, error) {
		return &slackStmt{numArgs: 2, execFn: func(args []driver.Value) (driver.Result, error) {
			return &slackResult{}, nil
		}}, nil
	})
	defer db.Close()

	s := NewStore(db)
	err := s.UpdateWorkspaceNotificationChannel(context.Background(), "T123", "C456")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// UpdateWorkspaceDefaultAgent
// ---------------------------------------------------------------------------

func TestUpdateWorkspaceDefaultAgent_Success(t *testing.T) {
	db := newSlackDB(func(query string) (driver.Stmt, error) {
		return &slackStmt{numArgs: 2, execFn: func(args []driver.Value) (driver.Result, error) {
			return &slackResult{}, nil
		}}, nil
	})
	defer db.Close()

	s := NewStore(db)
	err := s.UpdateWorkspaceDefaultAgent(context.Background(), "T123", uuid.New())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GetChannelBinding
// ---------------------------------------------------------------------------

func TestGetChannelBinding_NoRows(t *testing.T) {
	db := newSlackDB(func(query string) (driver.Stmt, error) {
		return &slackStmt{
			numArgs: 1,
			queryFn: func(args []driver.Value) (driver.Rows, error) {
				return &slackRows{cols: []string{"agent_id"}, values: nil}, nil
			},
		}, nil
	})
	defer db.Close()

	s := NewStore(db)
	id, err := s.GetChannelBinding(context.Background(), "C-NOTFOUND")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != nil {
		t.Errorf("expected nil, got %v", id)
	}
}

func TestGetChannelBinding_DBError(t *testing.T) {
	db := newSlackDB(func(query string) (driver.Stmt, error) {
		return nil, fmt.Errorf("db error")
	})
	defer db.Close()

	s := NewStore(db)
	_, err := s.GetChannelBinding(context.Background(), "C123")
	if err == nil {
		t.Error("expected error")
	}
}

func TestGetChannelBinding_Found(t *testing.T) {
	agentID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	db := newSlackDB(func(query string) (driver.Stmt, error) {
		return &slackStmt{
			numArgs: 1,
			queryFn: func(args []driver.Value) (driver.Rows, error) {
				return &slackRows{
					cols:   []string{"agent_id"},
					values: [][]driver.Value{{agentID.String()}},
				}, nil
			},
		}, nil
	})
	defer db.Close()

	s := NewStore(db)
	got, err := s.GetChannelBinding(context.Background(), "C123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil UUID")
	}
	if *got != agentID {
		t.Errorf("got %v, want %v", *got, agentID)
	}
}

// ---------------------------------------------------------------------------
// GetUserMapping
// ---------------------------------------------------------------------------

func TestGetUserMapping_NoRows(t *testing.T) {
	db := newSlackDB(func(query string) (driver.Stmt, error) {
		return &slackStmt{
			numArgs: 1,
			queryFn: func(args []driver.Value) (driver.Rows, error) {
				return &slackRows{cols: []string{"astra_user_id"}, values: nil}, nil
			},
		}, nil
	})
	defer db.Close()

	s := NewStore(db)
	id, err := s.GetUserMapping(context.Background(), "U-NOTFOUND")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != nil {
		t.Errorf("expected nil, got %v", id)
	}
}

func TestGetUserMapping_DBError(t *testing.T) {
	db := newSlackDB(func(query string) (driver.Stmt, error) {
		return nil, fmt.Errorf("db error")
	})
	defer db.Close()

	s := NewStore(db)
	_, err := s.GetUserMapping(context.Background(), "U123")
	if err == nil {
		t.Error("expected error")
	}
}

func TestGetUserMapping_Found(t *testing.T) {
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	db := newSlackDB(func(query string) (driver.Stmt, error) {
		return &slackStmt{
			numArgs: 1,
			queryFn: func(args []driver.Value) (driver.Rows, error) {
				return &slackRows{
					cols:   []string{"astra_user_id"},
					values: [][]driver.Value{{userID.String()}},
				}, nil
			},
		}, nil
	})
	defer db.Close()

	s := NewStore(db)
	got, err := s.GetUserMapping(context.Background(), "U123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || *got != userID {
		t.Errorf("got %v, want %v", got, userID)
	}
}

// ---------------------------------------------------------------------------
// GetConfig / SetConfig
// ---------------------------------------------------------------------------

func TestGetConfig_NoRows(t *testing.T) {
	db := newSlackDB(func(query string) (driver.Stmt, error) {
		return &slackStmt{
			numArgs: 1,
			queryFn: func(args []driver.Value) (driver.Rows, error) {
				return &slackRows{cols: []string{"value_encrypted"}, values: nil}, nil
			},
		}, nil
	})
	defer db.Close()

	s := NewStore(db)
	val, err := s.GetConfig(context.Background(), "signing_secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string, got %q", val)
	}
}

func TestGetConfig_PrepareError_ReturnsEmpty(t *testing.T) {
	// GetConfig checks !val.Valid before err != nil, so a prepare error where
	// val remains zero-value (Valid=false) returns ("", nil) — not an error.
	db := newSlackDB(func(query string) (driver.Stmt, error) {
		return nil, fmt.Errorf("db error")
	})
	defer db.Close()

	s := NewStore(db)
	val, err := s.GetConfig(context.Background(), "key")
	if err != nil {
		t.Errorf("GetConfig with prepare error: unexpected error %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string, got %q", val)
	}
}

func TestSetConfig_Success(t *testing.T) {
	db := newSlackDB(func(query string) (driver.Stmt, error) {
		return &slackStmt{numArgs: 2, execFn: func(args []driver.Value) (driver.Result, error) {
			return &slackResult{}, nil
		}}, nil
	})
	defer db.Close()

	s := NewStore(db)
	err := s.SetConfig(context.Background(), "signing_secret", "mysecret")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSetConfig_DBError(t *testing.T) {
	db := newSlackDB(func(query string) (driver.Stmt, error) {
		return nil, fmt.Errorf("db error")
	})
	defer db.Close()

	s := NewStore(db)
	err := s.SetConfig(context.Background(), "key", "val")
	if err == nil {
		t.Error("expected error")
	}
}

// ---------------------------------------------------------------------------
// GetSlackSessionByThread / CreateSlackSession
// ---------------------------------------------------------------------------

func TestGetSlackSessionByThread_NoRows(t *testing.T) {
	db := newSlackDB(func(query string) (driver.Stmt, error) {
		return &slackStmt{
			numArgs: 4,
			queryFn: func(args []driver.Value) (driver.Rows, error) {
				return &slackRows{cols: []string{"chat_session_id"}, values: nil}, nil
			},
		}, nil
	})
	defer db.Close()

	s := NewStore(db)
	id, err := s.GetSlackSessionByThread(context.Background(), "WS1", "C1", "U1", "ts123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != nil {
		t.Errorf("expected nil, got %v", id)
	}
}

func TestGetSlackSessionByThread_EmptyThreadTs(t *testing.T) {
	db := newSlackDB(func(query string) (driver.Stmt, error) {
		return &slackStmt{
			numArgs: 4,
			queryFn: func(args []driver.Value) (driver.Rows, error) {
				return &slackRows{cols: []string{"chat_session_id"}, values: nil}, nil
			},
		}, nil
	})
	defer db.Close()

	s := NewStore(db)
	// empty threadTs should be treated as RootThreadTS
	_, err := s.GetSlackSessionByThread(context.Background(), "WS1", "C1", "U1", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetSlackSessionByThread_DBError(t *testing.T) {
	db := newSlackDB(func(query string) (driver.Stmt, error) {
		return nil, fmt.Errorf("db error")
	})
	defer db.Close()

	s := NewStore(db)
	_, err := s.GetSlackSessionByThread(context.Background(), "W", "C", "U", "ts")
	if err == nil {
		t.Error("expected error")
	}
}

func TestCreateSlackSession_Success(t *testing.T) {
	db := newSlackDB(func(query string) (driver.Stmt, error) {
		return &slackStmt{numArgs: 5, execFn: func(args []driver.Value) (driver.Result, error) {
			return &slackResult{}, nil
		}}, nil
	})
	defer db.Close()

	s := NewStore(db)
	err := s.CreateSlackSession(context.Background(), uuid.New(), "WS1", "C1", "U1", "ts123")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCreateSlackSession_EmptyThreadTs(t *testing.T) {
	db := newSlackDB(func(query string) (driver.Stmt, error) {
		return &slackStmt{numArgs: 5, execFn: func(args []driver.Value) (driver.Result, error) {
			return &slackResult{}, nil
		}}, nil
	})
	defer db.Close()

	s := NewStore(db)
	err := s.CreateSlackSession(context.Background(), uuid.New(), "WS1", "C1", "U1", "")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCreateSlackSession_DBError(t *testing.T) {
	db := newSlackDB(func(query string) (driver.Stmt, error) {
		return nil, fmt.Errorf("db error")
	})
	defer db.Close()

	s := NewStore(db)
	err := s.CreateSlackSession(context.Background(), uuid.New(), "W", "C", "U", "ts")
	if err == nil {
		t.Error("expected error")
	}
}
