package rbac

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"testing"
)

// ---------------------------------------------------------------------------
// Minimal in-process SQL driver for testing
// ---------------------------------------------------------------------------

// mockDriver implements driver.Driver using a factory func.
type mockDriver struct {
	open func(name string) (driver.Conn, error)
}

func (d *mockDriver) Open(name string) (driver.Conn, error) { return d.open(name) }

// mockConn implements driver.Conn.
type mockConn struct {
	prepare func(query string) (driver.Stmt, error)
}

func (c *mockConn) Prepare(query string) (driver.Stmt, error) { return c.prepare(query) }
func (c *mockConn) Close() error                              { return nil }
func (c *mockConn) Begin() (driver.Tx, error)                 { return nil, fmt.Errorf("no tx") }

// mockStmt implements driver.Stmt.
type mockStmt struct {
	query   func(args []driver.Value) (driver.Rows, error)
	numArgs int
}

func (s *mockStmt) Close() error                                    { return nil }
func (s *mockStmt) NumInput() int                                   { return s.numArgs }
func (s *mockStmt) Exec(_ []driver.Value) (driver.Result, error)    { return nil, fmt.Errorf("no exec") }
func (s *mockStmt) Query(args []driver.Value) (driver.Rows, error)  { return s.query(args) }

// mockRows implements driver.Rows returning a single bool column.
type mockBoolRows struct {
	cols    []string
	values  [][]driver.Value
	current int
}

func (r *mockBoolRows) Columns() []string { return r.cols }
func (r *mockBoolRows) Close() error      { return nil }
func (r *mockBoolRows) Next(dest []driver.Value) error {
	if r.current >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.current])
	r.current++
	return nil
}

var driverSeq int

// newMockDB registers a unique driver and returns an open *sql.DB backed by it.
func newMockDB(prepare func(query string) (driver.Stmt, error)) *sql.DB {
	driverSeq++
	name := fmt.Sprintf("mockdb-%d", driverSeq)
	sql.Register(name, &mockDriver{
		open: func(_ string) (driver.Conn, error) {
			return &mockConn{prepare: prepare}, nil
		},
	})
	db, err := sql.Open(name, "")
	if err != nil {
		panic(err)
	}
	return db
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestNewDBCollaboratorChecker(t *testing.T) {
	db := newMockDB(func(query string) (driver.Stmt, error) { return nil, fmt.Errorf("unused") })
	defer db.Close()
	c := NewDBCollaboratorChecker(db)
	if c == nil {
		t.Fatal("expected non-nil checker")
	}
	if c.db != db {
		t.Error("db field not set correctly")
	}
}

func TestIsCollaborator_True(t *testing.T) {
	db := newMockDB(func(query string) (driver.Stmt, error) {
		return &mockStmt{
			numArgs: 2,
			query: func(args []driver.Value) (driver.Rows, error) {
				return &mockBoolRows{
					cols:   []string{"exists"},
					values: [][]driver.Value{{true}},
				}, nil
			},
		}, nil
	})
	defer db.Close()

	c := NewDBCollaboratorChecker(db)
	got, err := c.IsCollaborator(context.Background(), "user-1", "agent-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Error("expected true")
	}
}

func TestIsCollaborator_False(t *testing.T) {
	db := newMockDB(func(query string) (driver.Stmt, error) {
		return &mockStmt{
			numArgs: 2,
			query: func(args []driver.Value) (driver.Rows, error) {
				return &mockBoolRows{
					cols:   []string{"exists"},
					values: [][]driver.Value{{false}},
				}, nil
			},
		}, nil
	})
	defer db.Close()

	c := NewDBCollaboratorChecker(db)
	got, err := c.IsCollaborator(context.Background(), "user-2", "agent-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Error("expected false")
	}
}

func TestIsCollaborator_DBError(t *testing.T) {
	db := newMockDB(func(query string) (driver.Stmt, error) {
		return nil, fmt.Errorf("connection refused")
	})
	defer db.Close()

	c := NewDBCollaboratorChecker(db)
	_, err := c.IsCollaborator(context.Background(), "user-1", "agent-1")
	if err == nil {
		t.Error("expected error from DB failure")
	}
}

func TestIsCollaboratorViaTeam_EmptyTeamIDs(t *testing.T) {
	// Should short-circuit without hitting DB
	db := newMockDB(func(query string) (driver.Stmt, error) {
		t.Error("should not hit DB with empty teamIDs")
		return nil, fmt.Errorf("should not be called")
	})
	defer db.Close()

	c := NewDBCollaboratorChecker(db)
	got, err := c.IsCollaboratorViaTeam(context.Background(), []string{}, "agent-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Error("expected false for empty team IDs")
	}
}

func TestIsCollaboratorViaTeam_True(t *testing.T) {
	db := newMockDB(func(query string) (driver.Stmt, error) {
		return &mockStmt{
			numArgs: 2,
			query: func(args []driver.Value) (driver.Rows, error) {
				return &mockBoolRows{
					cols:   []string{"exists"},
					values: [][]driver.Value{{true}},
				}, nil
			},
		}, nil
	})
	defer db.Close()

	c := NewDBCollaboratorChecker(db)
	got, err := c.IsCollaboratorViaTeam(context.Background(), []string{"team-1", "team-2"}, "agent-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Error("expected true")
	}
}

func TestIsCollaboratorViaTeam_False(t *testing.T) {
	db := newMockDB(func(query string) (driver.Stmt, error) {
		return &mockStmt{
			numArgs: 2,
			query: func(args []driver.Value) (driver.Rows, error) {
				return &mockBoolRows{
					cols:   []string{"exists"},
					values: [][]driver.Value{{false}},
				}, nil
			},
		}, nil
	})
	defer db.Close()

	c := NewDBCollaboratorChecker(db)
	got, err := c.IsCollaboratorViaTeam(context.Background(), []string{"team-1"}, "agent-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Error("expected false")
	}
}

func TestIsCollaboratorViaTeam_DBError(t *testing.T) {
	db := newMockDB(func(query string) (driver.Stmt, error) {
		return nil, fmt.Errorf("db error")
	})
	defer db.Close()

	c := NewDBCollaboratorChecker(db)
	_, err := c.IsCollaboratorViaTeam(context.Background(), []string{"team-1"}, "agent-1")
	if err == nil {
		t.Error("expected error")
	}
}

func TestIsCollaboratorViaTeam_MultipleTeams(t *testing.T) {
	// Verify the array string is built correctly (no panic with multiple IDs)
	db := newMockDB(func(query string) (driver.Stmt, error) {
		return &mockStmt{
			numArgs: 2,
			query: func(args []driver.Value) (driver.Rows, error) {
				return &mockBoolRows{
					cols:   []string{"exists"},
					values: [][]driver.Value{{false}},
				}, nil
			},
		}, nil
	})
	defer db.Close()

	c := NewDBCollaboratorChecker(db)
	_, err := c.IsCollaboratorViaTeam(context.Background(), []string{"t1", "t2", "t3"}, "agent-x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
