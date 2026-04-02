package identity

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"sync"
	"testing"

	"github.com/google/uuid"
)

// ---- minimal sql mock driver ----

type mockDriver struct {
	mu   sync.Mutex
	conn *mockConn
}

func (d *mockDriver) Open(_ string) (driver.Conn, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.conn, nil
}

type mockConn struct {
	mu        sync.Mutex
	execFunc  func(query string, args []driver.Value) (driver.Result, error)
	queryFunc func(query string, args []driver.Value) (driver.Rows, error)
}

func (c *mockConn) Prepare(query string) (driver.Stmt, error) {
	return &mockStmt{conn: c, query: query}, nil
}

func (c *mockConn) Close() error { return nil }

func (c *mockConn) Begin() (driver.Tx, error) { return &mockTx{}, nil }

type mockTx struct{}

func (t *mockTx) Commit() error   { return nil }
func (t *mockTx) Rollback() error { return nil }

type mockStmt struct {
	conn  *mockConn
	query string
}

func (s *mockStmt) Close() error    { return nil }
func (s *mockStmt) NumInput() int   { return -1 }

func (s *mockStmt) Exec(args []driver.Value) (driver.Result, error) {
	if s.conn.execFunc != nil {
		return s.conn.execFunc(s.query, args)
	}
	return driver.RowsAffected(1), nil
}

func (s *mockStmt) Query(args []driver.Value) (driver.Rows, error) {
	if s.conn.queryFunc != nil {
		return s.conn.queryFunc(s.query, args)
	}
	return &mockRows{cols: []string{}}, nil
}

type mockRows struct {
	cols   []string
	data   [][]driver.Value
	pos    int
}

func (r *mockRows) Columns() []string { return r.cols }
func (r *mockRows) Close() error      { return nil }

func (r *mockRows) Next(dest []driver.Value) error {
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

var driverSeq int
var driverMu sync.Mutex

func newMockDB(conn *mockConn) *sql.DB {
	driverMu.Lock()
	driverSeq++
	name := fmt.Sprintf("identity_mock_%d", driverSeq)
	driverMu.Unlock()
	sql.Register(name, &mockDriver{conn: conn})
	db, _ := sql.Open(name, "mock")
	return db
}

func TestNewStore(t *testing.T) {
	db := newMockDB(&mockConn{})
	defer db.Close()
	s := NewStore(db)
	if s == nil {
		t.Fatal("NewStore returned nil")
	}
}

func TestGetUserByIDNotFound(t *testing.T) {
	conn := &mockConn{
		queryFunc: func(_ string, _ []driver.Value) (driver.Rows, error) {
			return &mockRows{
				cols: []string{"id", "email", "name", "password_hash", "status", "is_super_admin", "last_login_at", "created_at", "updated_at"},
				data: nil,
			}, nil
		},
	}
	db := newMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	user, err := s.GetUserByID(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if user != nil {
		t.Error("expected nil user for not found")
	}
}

func TestGetUserByEmailNotFound(t *testing.T) {
	conn := &mockConn{
		queryFunc: func(_ string, _ []driver.Value) (driver.Rows, error) {
			return &mockRows{
				cols: []string{"id", "email", "name", "password_hash", "status", "is_super_admin", "last_login_at", "created_at", "updated_at"},
				data: nil,
			}, nil
		},
	}
	db := newMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	user, err := s.GetUserByEmail(context.Background(), "missing@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if user != nil {
		t.Error("expected nil user for not found")
	}
}

func TestGetUserByIDNotFoundExplicit(t *testing.T) {
	// Verify that a query returning zero rows gives nil, nil (not an error)
	conn := &mockConn{
		queryFunc: func(_ string, _ []driver.Value) (driver.Rows, error) {
			return &mockRows{
				cols: []string{"id", "email", "name", "password_hash", "status", "is_super_admin", "last_login_at", "created_at", "updated_at"},
				data: nil, // no rows
			}, nil
		},
	}
	db := newMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	id := uuid.New()
	user, err := s.GetUserByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetUserByID: unexpected error: %v", err)
	}
	if user != nil {
		t.Errorf("expected nil user, got %+v", user)
	}
}

func TestDeleteUserExec(t *testing.T) {
	var execCalled bool
	conn := &mockConn{
		execFunc: func(_ string, _ []driver.Value) (driver.Result, error) {
			execCalled = true
			return driver.RowsAffected(1), nil
		},
	}
	db := newMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	err := s.DeleteUser(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if !execCalled {
		t.Error("expected exec to be called")
	}
}

func TestUpdateLastLoginExec(t *testing.T) {
	var execCalled bool
	conn := &mockConn{
		execFunc: func(_ string, _ []driver.Value) (driver.Result, error) {
			execCalled = true
			return driver.RowsAffected(1), nil
		},
	}
	db := newMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	err := s.UpdateLastLogin(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("UpdateLastLogin: %v", err)
	}
	if !execCalled {
		t.Error("expected exec to be called")
	}
}

func TestUpdateUserNoFields(t *testing.T) {
	var execCalled bool
	conn := &mockConn{
		execFunc: func(_ string, _ []driver.Value) (driver.Result, error) {
			execCalled = true
			return driver.RowsAffected(1), nil
		},
	}
	db := newMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	// All nil fields → no-op
	err := s.UpdateUser(context.Background(), uuid.New(), nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("UpdateUser (no fields): %v", err)
	}
	if execCalled {
		t.Error("expected no exec call for zero-field update")
	}
}

func TestUpdateUserWithFields(t *testing.T) {
	var execCalled bool
	conn := &mockConn{
		execFunc: func(_ string, _ []driver.Value) (driver.Result, error) {
			execCalled = true
			return driver.RowsAffected(1), nil
		},
	}
	db := newMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	name := "Bob"
	err := s.UpdateUser(context.Background(), uuid.New(), &name, nil, nil, nil)
	if err != nil {
		t.Fatalf("UpdateUser (with name): %v", err)
	}
	if !execCalled {
		t.Error("expected exec to be called for name update")
	}
}

func TestAuthenticateUserNotFound(t *testing.T) {
	conn := &mockConn{
		queryFunc: func(_ string, _ []driver.Value) (driver.Rows, error) {
			return &mockRows{
				cols: []string{"id", "email", "name", "password_hash", "status", "is_super_admin", "last_login_at", "created_at", "updated_at"},
				data: nil,
			}, nil
		},
	}
	db := newMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	user, err := s.Authenticate(context.Background(), "nobody@example.com", "pass")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if user != nil {
		t.Error("expected nil for missing user")
	}
}

func TestListUsersEmptyResult(t *testing.T) {
	callCount := 0
	conn := &mockConn{
		queryFunc: func(query string, args []driver.Value) (driver.Rows, error) {
			callCount++
			if callCount == 1 {
				// COUNT query
				return &mockRows{
					cols: []string{"count"},
					data: [][]driver.Value{{int64(0)}},
				}, nil
			}
			// data query
			return &mockRows{
				cols: []string{"id", "email", "name", "password_hash", "status", "is_super_admin", "last_login_at", "created_at", "updated_at"},
				data: nil,
			}, nil
		},
	}
	db := newMockDB(conn)
	defer db.Close()
	s := NewStore(db)

	users, total, err := s.ListUsers(context.Background(), "", "", 10, 0)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if total != 0 {
		t.Errorf("total: got %d, want 0", total)
	}
	if len(users) != 0 {
		t.Errorf("users: got %d, want 0", len(users))
	}
}
