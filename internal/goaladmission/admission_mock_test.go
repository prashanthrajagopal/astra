package goaladmission

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
// Minimal in-process SQL driver
// ---------------------------------------------------------------------------

type gaDriver struct {
	open func(string) (driver.Conn, error)
}

func (d *gaDriver) Open(name string) (driver.Conn, error) { return d.open(name) }

type gaConn struct {
	prepare func(string) (driver.Stmt, error)
}

func (c *gaConn) Prepare(query string) (driver.Stmt, error) { return c.prepare(query) }
func (c *gaConn) Close() error                              { return nil }
func (c *gaConn) Begin() (driver.Tx, error)                 { return nil, fmt.Errorf("no tx") }

type gaStmt struct {
	query   func([]driver.Value) (driver.Rows, error)
	numArgs int
}

func (s *gaStmt) Close() error                                   { return nil }
func (s *gaStmt) NumInput() int                                  { return s.numArgs }
func (s *gaStmt) Exec([]driver.Value) (driver.Result, error)     { return nil, fmt.Errorf("no exec") }
func (s *gaStmt) Query(args []driver.Value) (driver.Rows, error) { return s.query(args) }

type gaRows struct {
	cols    []string
	values  [][]driver.Value
	current int
}

func (r *gaRows) Columns() []string { return r.cols }
func (r *gaRows) Close() error      { return nil }
func (r *gaRows) Next(dest []driver.Value) error {
	if r.current >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.current])
	r.current++
	return nil
}

var gaSeq int

func newGADB(prepare func(string) (driver.Stmt, error)) *sql.DB {
	gaSeq++
	name := fmt.Sprintf("gadb-%d", gaSeq)
	sql.Register(name, &gaDriver{
		open: func(_ string) (driver.Conn, error) {
			return &gaConn{prepare: prepare}, nil
		},
	})
	db, err := sql.Open(name, "")
	if err != nil {
		panic(err)
	}
	return db
}

// ---------------------------------------------------------------------------
// Tests for CheckBeforeNewGoal
// ---------------------------------------------------------------------------

func TestCheckBeforeNewGoal_DrainMode(t *testing.T) {
	agentID := uuid.New()
	callCount := 0
	db := newGADB(func(query string) (driver.Stmt, error) {
		callCount++
		// First query: SELECT drain_mode, max_concurrent_goals, daily_token_budget
		return &gaStmt{
			numArgs: 1,
			query: func(args []driver.Value) (driver.Rows, error) {
				return &gaRows{
					cols:   []string{"drain_mode", "max_concurrent_goals", "daily_token_budget"},
					values: [][]driver.Value{{true, nil, nil}},
				}, nil
			},
		}, nil
	})
	defer db.Close()

	err := CheckBeforeNewGoal(context.Background(), db, nil, agentID)
	if err != ErrDrainMode {
		t.Errorf("expected ErrDrainMode, got %v", err)
	}
}

func TestCheckBeforeNewGoal_DBError(t *testing.T) {
	agentID := uuid.New()
	db := newGADB(func(query string) (driver.Stmt, error) {
		return nil, fmt.Errorf("db connection failed")
	})
	defer db.Close()

	err := CheckBeforeNewGoal(context.Background(), db, nil, agentID)
	if err == nil {
		t.Error("expected error from DB failure")
	}
}

func TestCheckBeforeNewGoal_NoDrainNoCapNoBudget(t *testing.T) {
	agentID := uuid.New()
	db := newGADB(func(query string) (driver.Stmt, error) {
		return &gaStmt{
			numArgs: 1,
			query: func(args []driver.Value) (driver.Rows, error) {
				// drain=false, max_concurrent_goals=NULL, daily_token_budget=NULL
				return &gaRows{
					cols:   []string{"drain_mode", "max_concurrent_goals", "daily_token_budget"},
					values: [][]driver.Value{{false, nil, nil}},
				}, nil
			},
		}, nil
	})
	defer db.Close()

	err := CheckBeforeNewGoal(context.Background(), db, nil, agentID)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Tests for IncrAgentDailyTokens (nil-safe paths)
// ---------------------------------------------------------------------------

func TestIncrAgentDailyTokens_NilRedis(t *testing.T) {
	id := uuid.New()
	err := IncrAgentDailyTokens(context.Background(), nil, id, 100)
	if err != nil {
		t.Errorf("nil redis should be no-op, got %v", err)
	}
}

func TestIncrAgentDailyTokens_ZeroTokens(t *testing.T) {
	id := uuid.New()
	err := IncrAgentDailyTokens(context.Background(), nil, id, 0)
	if err != nil {
		t.Errorf("zero tokens should be no-op, got %v", err)
	}
}

func TestIncrAgentDailyTokens_NegativeTokens(t *testing.T) {
	id := uuid.New()
	err := IncrAgentDailyTokens(context.Background(), nil, id, -1)
	if err != nil {
		t.Errorf("negative tokens should be no-op, got %v", err)
	}
}

func TestIncrAgentDailyTokens_NilUUID(t *testing.T) {
	err := IncrAgentDailyTokens(context.Background(), nil, uuid.Nil, 100)
	if err != nil {
		t.Errorf("nil UUID should be no-op, got %v", err)
	}
}
