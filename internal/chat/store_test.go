package chat

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"

	"github.com/google/uuid"
)

// ---- minimal sql mock driver ----

type chatMockDriver struct {
	conn *chatMockConn
}

func (d *chatMockDriver) Open(_ string) (driver.Conn, error) {
	return d.conn, nil
}

type chatMockConn struct {
	execFunc  func(query string, args []driver.Value) (driver.Result, error)
	queryFunc func(query string, args []driver.Value) (driver.Rows, error)
}

func (c *chatMockConn) Prepare(query string) (driver.Stmt, error) {
	return &chatMockStmt{conn: c, query: query}, nil
}

func (c *chatMockConn) Close() error                        { return nil }
func (c *chatMockConn) Begin() (driver.Tx, error)           { return &chatMockTx{}, nil }

type chatMockTx struct{}

func (t *chatMockTx) Commit() error   { return nil }
func (t *chatMockTx) Rollback() error { return nil }

type chatMockStmt struct {
	conn  *chatMockConn
	query string
}

func (s *chatMockStmt) Close() error  { return nil }
func (s *chatMockStmt) NumInput() int { return -1 }

func (s *chatMockStmt) Exec(args []driver.Value) (driver.Result, error) {
	if s.conn.execFunc != nil {
		return s.conn.execFunc(s.query, args)
	}
	return driver.RowsAffected(1), nil
}

func (s *chatMockStmt) Query(args []driver.Value) (driver.Rows, error) {
	if s.conn.queryFunc != nil {
		return s.conn.queryFunc(s.query, args)
	}
	return &chatMockRows{cols: []string{}}, nil
}

type chatMockRows struct {
	cols []string
	data [][]driver.Value
	pos  int
}

func (r *chatMockRows) Columns() []string { return r.cols }
func (r *chatMockRows) Close() error      { return nil }

func (r *chatMockRows) Next(dest []driver.Value) error {
	if r.pos >= len(r.data) {
		return io.EOF
	}
	for i, v := range r.data[r.pos] {
		if i < len(dest) {
			dest[i] = v
		}
	}
	r.pos++
	return nil
}

var chatDriverSeq int
var chatDriverMu sync.Mutex

func newChatMockDB(conn *chatMockConn) *sql.DB {
	chatDriverMu.Lock()
	chatDriverSeq++
	name := fmt.Sprintf("chat_mock_%d", chatDriverSeq)
	chatDriverMu.Unlock()
	sql.Register(name, &chatMockDriver{conn: conn})
	db, _ := sql.Open(name, "mock")
	return db
}

var errChat = errors.New("chat db error")

func TestChatNewStore(t *testing.T) {
	db := newChatMockDB(&chatMockConn{})
	defer db.Close()
	s := NewStore(db)
	if s == nil {
		t.Fatal("NewStore returned nil")
	}
}

func TestGetSessionNotFound(t *testing.T) {
	conn := &chatMockConn{
		queryFunc: func(_ string, _ []driver.Value) (driver.Rows, error) {
			return &chatMockRows{
				cols: []string{"id", "user_id", "agent_id", "title", "status", "created_at", "updated_at", "expires_at"},
				data: nil, // no rows → ErrNoRows
			}, nil
		},
	}
	db := newChatMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	session, err := s.GetSession(context.Background(), uuid.New(), "user-1")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if session != nil {
		t.Error("expected nil session for not found")
	}
}

func TestGetSessionQueryError(t *testing.T) {
	conn := &chatMockConn{
		queryFunc: func(_ string, _ []driver.Value) (driver.Rows, error) {
			return nil, errChat
		},
	}
	db := newChatMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	_, err := s.GetSession(context.Background(), uuid.New(), "user-1")
	if err == nil {
		t.Error("expected error from query failure")
	}
}

func TestListSessionsEmpty(t *testing.T) {
	conn := &chatMockConn{
		queryFunc: func(_ string, _ []driver.Value) (driver.Rows, error) {
			return &chatMockRows{
				cols: []string{"id", "user_id", "agent_id", "title", "status", "created_at", "updated_at", "expires_at"},
				data: nil,
			}, nil
		},
	}
	db := newChatMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	sessions, err := s.ListSessions(context.Background(), "user-1", nil)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestListSessionsWithAgentFilter(t *testing.T) {
	conn := &chatMockConn{
		queryFunc: func(_ string, _ []driver.Value) (driver.Rows, error) {
			return &chatMockRows{
				cols: []string{"id", "user_id", "agent_id", "title", "status", "created_at", "updated_at", "expires_at"},
				data: nil,
			}, nil
		},
	}
	db := newChatMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	agentID := uuid.New()
	sessions, err := s.ListSessions(context.Background(), "user-1", &agentID)
	if err != nil {
		t.Fatalf("ListSessions with agent filter: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestListSessionsQueryError(t *testing.T) {
	conn := &chatMockConn{
		queryFunc: func(_ string, _ []driver.Value) (driver.Rows, error) {
			return nil, errChat
		},
	}
	db := newChatMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	_, err := s.ListSessions(context.Background(), "user-1", nil)
	if err == nil {
		t.Error("expected error from query failure")
	}
}

func TestGetSessionTokenTotalZero(t *testing.T) {
	conn := &chatMockConn{
		queryFunc: func(_ string, _ []driver.Value) (driver.Rows, error) {
			return &chatMockRows{
				cols: []string{"total"},
				data: [][]driver.Value{{int64(0)}},
			}, nil
		},
	}
	db := newChatMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	total, err := s.GetSessionTokenTotal(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("GetSessionTokenTotal: %v", err)
	}
	if total != 0 {
		t.Errorf("total: got %d, want 0", total)
	}
}

func TestGetSessionTokenTotalError(t *testing.T) {
	conn := &chatMockConn{
		queryFunc: func(_ string, _ []driver.Value) (driver.Rows, error) {
			return nil, errChat
		},
	}
	db := newChatMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	_, err := s.GetSessionTokenTotal(context.Background(), uuid.New())
	if err == nil {
		t.Error("expected error from query failure")
	}
}

func TestGetMessagesSessionNotFound(t *testing.T) {
	conn := &chatMockConn{
		queryFunc: func(_ string, _ []driver.Value) (driver.Rows, error) {
			// session check returns no rows
			return &chatMockRows{
				cols: []string{"exists"},
				data: nil,
			}, nil
		},
	}
	db := newChatMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	msgs, err := s.GetMessages(context.Background(), uuid.New(), "user-1")
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if msgs != nil {
		t.Error("expected nil messages for missing session")
	}
}

func TestGetMessagesSessionCheckError(t *testing.T) {
	conn := &chatMockConn{
		queryFunc: func(_ string, _ []driver.Value) (driver.Rows, error) {
			return nil, errChat
		},
	}
	db := newChatMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	_, err := s.GetMessages(context.Background(), uuid.New(), "user-1")
	if err == nil {
		t.Error("expected error from session check failure")
	}
}

func TestListChatCapableAgentsEmpty(t *testing.T) {
	conn := &chatMockConn{
		queryFunc: func(_ string, _ []driver.Value) (driver.Rows, error) {
			return &chatMockRows{
				cols: []string{"id", "name"},
				data: nil,
			}, nil
		},
	}
	db := newChatMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	agents, err := s.ListChatCapableAgents(context.Background())
	if err != nil {
		t.Fatalf("ListChatCapableAgents: %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(agents))
	}
}

func TestListChatCapableAgentsQueryError(t *testing.T) {
	conn := &chatMockConn{
		queryFunc: func(_ string, _ []driver.Value) (driver.Rows, error) {
			return nil, errChat
		},
	}
	db := newChatMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	_, err := s.ListChatCapableAgents(context.Background())
	if err == nil {
		t.Error("expected error from query failure")
	}
}

func TestAppendMessageSessionNotFound(t *testing.T) {
	conn := &chatMockConn{
		queryFunc: func(_ string, _ []driver.Value) (driver.Rows, error) {
			// session check: no rows
			return &chatMockRows{
				cols: []string{"exists"},
				data: nil,
			}, nil
		},
	}
	db := newChatMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	msg, err := s.AppendMessage(context.Background(), uuid.New(), "user-1", "user", "hello", nil, nil, 0, 0)
	if err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	if msg != nil {
		t.Error("expected nil message for missing session")
	}
}

func TestAppendMessageSessionCheckError(t *testing.T) {
	conn := &chatMockConn{
		queryFunc: func(_ string, _ []driver.Value) (driver.Rows, error) {
			return nil, errChat
		},
	}
	db := newChatMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	_, err := s.AppendMessage(context.Background(), uuid.New(), "user-1", "user", "hello", nil, nil, 0, 0)
	if err == nil {
		t.Error("expected error from session check failure")
	}
}
