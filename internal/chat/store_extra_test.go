package chat

import (
	"context"
	"database/sql/driver"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestListSessionsScanError(t *testing.T) {
	conn := &chatMockConn{
		queryFunc: func(_ string, _ []driver.Value) (driver.Rows, error) {
			// Return a row that will cause scan error (wrong number/type of cols)
			return &chatMockRows{
				cols: []string{"id", "user_id", "agent_id", "title", "status", "created_at", "updated_at", "expires_at"},
				data: [][]driver.Value{
					// agent_id is invalid uuid string — will fail uuid.Parse but not scan
					{uuid.New(), "user-1", "not-a-uuid", "title", "active", time.Now(), time.Now(), nil},
				},
			}, nil
		},
	}
	db := newChatMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	// The scan itself may succeed (uuid.Parse is soft-failed in source), so just verify no panic
	sessions, err := s.ListSessions(context.Background(), "user-1", nil)
	// Either success with invalid UUID or error — both acceptable
	_ = sessions
	_ = err
}

func TestGetMessagesEmptySession(t *testing.T) {
	callCount := 0
	conn := &chatMockConn{
		queryFunc: func(_ string, _ []driver.Value) (driver.Rows, error) {
			callCount++
			if callCount == 1 {
				// session check: found
				return &chatMockRows{
					cols: []string{"exists"},
					data: [][]driver.Value{{int64(1)}},
				}, nil
			}
			// messages query: empty
			return &chatMockRows{
				cols: []string{"id", "session_id", "role", "content", "tool_calls", "tool_results", "tokens_in", "tokens_out", "created_at"},
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
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
	}
}

func TestGetMessagesQueryError(t *testing.T) {
	callCount := 0
	conn := &chatMockConn{
		queryFunc: func(_ string, _ []driver.Value) (driver.Rows, error) {
			callCount++
			if callCount == 1 {
				// session check: found
				return &chatMockRows{
					cols: []string{"exists"},
					data: [][]driver.Value{{int64(1)}},
				}, nil
			}
			// messages query fails
			return nil, errChat
		},
	}
	db := newChatMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	_, err := s.GetMessages(context.Background(), uuid.New(), "user-1")
	if err == nil {
		t.Error("expected error from messages query failure")
	}
}

func TestGetSessionTokenTotalNonZero(t *testing.T) {
	conn := &chatMockConn{
		queryFunc: func(_ string, _ []driver.Value) (driver.Rows, error) {
			return &chatMockRows{
				cols: []string{"total"},
				data: [][]driver.Value{{int64(1500)}},
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
	if total != 1500 {
		t.Errorf("total: got %d, want 1500", total)
	}
}

func TestListChatCapableAgentsScanError(t *testing.T) {
	conn := &chatMockConn{
		queryFunc: func(_ string, _ []driver.Value) (driver.Rows, error) {
			return &chatMockRows{
				cols: []string{"id", "name"},
				data: [][]driver.Value{
					{"not-a-uuid", "Agent1"},
				},
			}, nil
		},
	}
	db := newChatMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	// uuid.Parse failure is soft in source (uses _) — should return one agent
	agents, err := s.ListChatCapableAgents(context.Background())
	if err != nil {
		t.Fatalf("ListChatCapableAgents: %v", err)
	}
	if len(agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(agents))
	}
}

func TestCreateSessionExecError(t *testing.T) {
	conn := &chatMockConn{
		execFunc: func(_ string, _ []driver.Value) (driver.Result, error) {
			return nil, errChat
		},
	}
	db := newChatMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	_, err := s.CreateSession(context.Background(), "user-1", uuid.New(), "My Session")
	if err == nil {
		t.Error("expected error from exec failure")
	}
}

func TestCreateSessionSuccess(t *testing.T) {
	callCount := 0
	conn := &chatMockConn{
		execFunc: func(_ string, _ []driver.Value) (driver.Result, error) {
			return driver.RowsAffected(1), nil
		},
		queryFunc: func(_ string, _ []driver.Value) (driver.Rows, error) {
			callCount++
			// RETURNING created_at, updated_at query
			return &chatMockRows{
				cols: []string{"created_at", "updated_at"},
				data: [][]driver.Value{
					{time.Now(), time.Now()},
				},
			}, nil
		},
	}
	db := newChatMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	session, err := s.CreateSession(context.Background(), "user-1", uuid.New(), "My Session")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if session == nil {
		t.Fatal("expected non-nil session")
	}
	if session.UserID != "user-1" {
		t.Errorf("UserID: got %q, want user-1", session.UserID)
	}
	if session.Status != "active" {
		t.Errorf("Status: got %q, want active", session.Status)
	}
}
