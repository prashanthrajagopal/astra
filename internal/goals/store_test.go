package goals

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
	mu       sync.Mutex
	stmts    map[string]*mockStmt
	execFunc func(query string, args []driver.Value) (driver.Result, error)
	queryFunc func(query string, args []driver.Value) (driver.Rows, error)
}

func (c *mockConn) Prepare(query string) (driver.Stmt, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stmts == nil {
		c.stmts = make(map[string]*mockStmt)
	}
	s := &mockStmt{conn: c, query: query}
	c.stmts[query] = s
	return s, nil
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

func (s *mockStmt) Close() error { return nil }

func (s *mockStmt) NumInput() int { return -1 }

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
	return &mockRows{cols: []string{}, data: nil}, nil
}

type mockRows struct {
	cols    []string
	data    [][]driver.Value
	pos     int
	closed  bool
}

func (r *mockRows) Columns() []string { return r.cols }

func (r *mockRows) Close() error {
	r.closed = true
	return nil
}

func (r *mockRows) Next(dest []driver.Value) error {
	if r.pos >= len(r.data) {
		return io.EOF
	}
	row := r.data[r.pos]
	r.pos++
	for i, v := range row {
		if i < len(dest) {
			dest[i] = v
		}
	}
	return nil
}

// registerMockDriver registers a unique driver name and returns a *sql.DB backed by it.
var driverSeq int
var driverMu sync.Mutex

func newMockDB(conn *mockConn) (*sql.DB, string) {
	driverMu.Lock()
	driverSeq++
	name := fmt.Sprintf("goals_mock_%d", driverSeq)
	driverMu.Unlock()
	sql.Register(name, &mockDriver{conn: conn})
	db, _ := sql.Open(name, "mock")
	return db, name
}

// ---- mockBus for messaging.Bus ----

type mockBus struct {
	mu           sync.Mutex
	publishedTo  []string
	publishedFields []map[string]interface{}
	publishErr   error
}

func (b *mockBus) Publish(_ context.Context, stream string, fields map[string]interface{}) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.publishErr != nil {
		return b.publishErr
	}
	b.publishedTo = append(b.publishedTo, stream)
	b.publishedFields = append(b.publishedFields, fields)
	return nil
}

// ---- tests ----

func TestNewStoreNotNil(t *testing.T) {
	conn := &mockConn{}
	db, _ := newMockDB(conn)
	defer db.Close()
	s := NewStore(db, nil)
	if s == nil {
		t.Fatal("NewStore returned nil")
	}
}

func TestAllDepsCompletedEmptyList(t *testing.T) {
	conn := &mockConn{}
	db, _ := newMockDB(conn)
	defer db.Close()
	s := NewStore(db, nil)

	ok, err := s.allDepsCompleted(context.Background(), nil)
	if err != nil {
		t.Fatalf("allDepsCompleted: %v", err)
	}
	if !ok {
		t.Error("expected true for empty deps list")
	}
}

func TestAllDepsCompletedAllDone(t *testing.T) {
	conn := &mockConn{
		queryFunc: func(query string, args []driver.Value) (driver.Rows, error) {
			// SELECT COUNT(*) = 0 → true (all completed)
			return &mockRows{
				cols: []string{"?column?"},
				data: [][]driver.Value{{true}},
			}, nil
		},
	}
	db, _ := newMockDB(conn)
	defer db.Close()
	s := NewStore(db, nil)

	ids := []uuid.UUID{uuid.New()}
	ok, err := s.allDepsCompleted(context.Background(), ids)
	if err != nil {
		t.Fatalf("allDepsCompleted: %v", err)
	}
	if !ok {
		t.Error("expected true when all deps completed")
	}
}

func TestAllDepsCompletedNotAllDone(t *testing.T) {
	conn := &mockConn{
		queryFunc: func(query string, args []driver.Value) (driver.Rows, error) {
			// COUNT(*) = 0 → false (some not completed)
			return &mockRows{
				cols: []string{"?column?"},
				data: [][]driver.Value{{false}},
			}, nil
		},
	}
	db, _ := newMockDB(conn)
	defer db.Close()
	s := NewStore(db, nil)

	ids := []uuid.UUID{uuid.New()}
	ok, err := s.allDepsCompleted(context.Background(), ids)
	if err != nil {
		t.Fatalf("allDepsCompleted: %v", err)
	}
	if ok {
		t.Error("expected false when not all deps completed")
	}
}

func TestCheckAndActivateBlockedNoRows(t *testing.T) {
	conn := &mockConn{
		queryFunc: func(query string, args []driver.Value) (driver.Rows, error) {
			// No blocked goals depend on this one
			return &mockRows{cols: []string{"id", "depends_on_goal_ids"}, data: nil}, nil
		},
	}
	db, _ := newMockDB(conn)
	defer db.Close()
	s := NewStore(db, nil)

	err := s.CheckAndActivateBlocked(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("CheckAndActivateBlocked: %v", err)
	}
}
