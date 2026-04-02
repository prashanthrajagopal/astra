package goaladmission

import (
	"context"
	"database/sql/driver"
	"fmt"
	"io"
	"testing"

	"github.com/google/uuid"
)

func gaScalarRows(val driver.Value) *gaRows {
	return &gaRows{cols: []string{"v"}, values: [][]driver.Value{{val}}}
}

func gaEOFRows() *gaRows {
	return &gaRows{cols: []string{"v"}, values: nil}
}

// ---------------------------------------------------------------------------
// Tests that exercise the concurrent-cap and budget paths
// ---------------------------------------------------------------------------

func TestCheckBeforeNewGoal_ConcurrentCapReached(t *testing.T) {
	agentID := uuid.New()
	callN := 0
	db := newGADB(func(query string) (driver.Stmt, error) {
		callN++
		switch callN {
		case 1:
			// SELECT drain_mode, max_concurrent_goals, daily_token_budget
			return &gaStmt{
				numArgs: 1,
				query: func(args []driver.Value) (driver.Rows, error) {
					return &gaRows{
						cols:   []string{"drain_mode", "max_concurrent_goals", "daily_token_budget"},
						values: [][]driver.Value{{false, int64(2), nil}},
					}, nil
				},
			}, nil
		case 2:
			// SELECT COUNT(*) FROM goals WHERE status IN ('pending','active')
			return &gaStmt{
				numArgs: 1,
				query: func(args []driver.Value) (driver.Rows, error) {
					return gaScalarRows(int64(2)), nil // at cap
				},
			}, nil
		default:
			return nil, fmt.Errorf("unexpected query %d", callN)
		}
	})
	defer db.Close()

	err := CheckBeforeNewGoal(context.Background(), db, nil, agentID)
	if err != ErrConcurrentCap {
		t.Errorf("expected ErrConcurrentCap, got %v", err)
	}
}

func TestCheckBeforeNewGoal_ConcurrentCapNotReached(t *testing.T) {
	agentID := uuid.New()
	callN := 0
	db := newGADB(func(query string) (driver.Stmt, error) {
		callN++
		switch callN {
		case 1:
			return &gaStmt{
				numArgs: 1,
				query: func(args []driver.Value) (driver.Rows, error) {
					// max_concurrent_goals=5, no daily budget
					return &gaRows{
						cols:   []string{"drain_mode", "max_concurrent_goals", "daily_token_budget"},
						values: [][]driver.Value{{false, int64(5), nil}},
					}, nil
				},
			}, nil
		case 2:
			// count = 3, below cap
			return &gaStmt{
				numArgs: 1,
				query: func(args []driver.Value) (driver.Rows, error) {
					return gaScalarRows(int64(3)), nil
				},
			}, nil
		default:
			return nil, fmt.Errorf("unexpected query %d", callN)
		}
	})
	defer db.Close()

	err := CheckBeforeNewGoal(context.Background(), db, nil, agentID)
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestCheckBeforeNewGoal_TokenBudgetExceededFromDB(t *testing.T) {
	agentID := uuid.New()
	callN := 0
	db := newGADB(func(query string) (driver.Stmt, error) {
		callN++
		switch callN {
		case 1:
			// drain=false, no max_conc, daily_budget=1000
			return &gaStmt{
				numArgs: 1,
				query: func(args []driver.Value) (driver.Rows, error) {
					return &gaRows{
						cols:   []string{"drain_mode", "max_concurrent_goals", "daily_token_budget"},
						values: [][]driver.Value{{false, nil, int64(1000)}},
					}, nil
				},
			}, nil
		case 2:
			// llm_usage query returns 1500 (over budget)
			return &gaStmt{
				numArgs: 1,
				query: func(args []driver.Value) (driver.Rows, error) {
					return gaScalarRows(int64(1500)), nil
				},
			}, nil
		default:
			return nil, fmt.Errorf("unexpected query %d", callN)
		}
	})
	defer db.Close()

	// rdb=nil so it falls to DB path
	err := CheckBeforeNewGoal(context.Background(), db, nil, agentID)
	if err != ErrTokenBudget {
		t.Errorf("expected ErrTokenBudget, got %v", err)
	}
}

func TestCheckBeforeNewGoal_TokenBudgetNotExceeded(t *testing.T) {
	agentID := uuid.New()
	callN := 0
	db := newGADB(func(query string) (driver.Stmt, error) {
		callN++
		switch callN {
		case 1:
			return &gaStmt{
				numArgs: 1,
				query: func(args []driver.Value) (driver.Rows, error) {
					return &gaRows{
						cols:   []string{"drain_mode", "max_concurrent_goals", "daily_token_budget"},
						values: [][]driver.Value{{false, nil, int64(1000)}},
					}, nil
				},
			}, nil
		case 2:
			// used = 500, under budget
			return &gaStmt{
				numArgs: 1,
				query: func(args []driver.Value) (driver.Rows, error) {
					return gaScalarRows(int64(500)), nil
				},
			}, nil
		default:
			return nil, fmt.Errorf("unexpected query %d", callN)
		}
	})
	defer db.Close()

	err := CheckBeforeNewGoal(context.Background(), db, nil, agentID)
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestCheckBeforeNewGoal_ZeroDailyBudget(t *testing.T) {
	agentID := uuid.New()
	db := newGADB(func(query string) (driver.Stmt, error) {
		return &gaStmt{
			numArgs: 1,
			query: func(args []driver.Value) (driver.Rows, error) {
				return &gaRows{
					cols:   []string{"drain_mode", "max_concurrent_goals", "daily_token_budget"},
					values: [][]driver.Value{{false, nil, int64(0)}},
				}, nil
			},
		}, nil
	})
	defer db.Close()

	// daily_budget <= 0 should return nil (no budget check)
	err := CheckBeforeNewGoal(context.Background(), db, nil, agentID)
	if err != nil {
		t.Errorf("expected nil for zero budget, got %v", err)
	}
}

// Ensure gaRows works with nil values sentinel (io.EOF).
func TestGaRows_EOF(t *testing.T) {
	r := gaEOFRows()
	dest := make([]driver.Value, 1)
	if err := r.Next(dest); err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}
}
