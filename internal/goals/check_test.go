package goals

import (
	"context"
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/google/uuid"
)

var errGoals = errors.New("goals db error")

func TestCheckAndActivateBlockedQueryError(t *testing.T) {
	conn := &mockConn{
		queryFunc: func(_ string, _ []driver.Value) (driver.Rows, error) {
			return nil, errGoals
		},
	}
	db, _ := newMockDB(conn)
	defer db.Close()
	s := NewStore(db, nil)

	err := s.CheckAndActivateBlocked(context.Background(), uuid.New())
	if err == nil {
		t.Error("expected error from query failure")
	}
}

func TestCheckAndActivateBlockedScanError(t *testing.T) {
	// Return a row but with a scan-incompatible type to trigger scan error
	conn := &mockConn{
		queryFunc: func(_ string, _ []driver.Value) (driver.Rows, error) {
			return &mockRows{
				cols: []string{"id", "depends_on_goal_ids"},
				data: [][]driver.Value{
					{"not-a-valid-uuid", "{}"},
				},
			}, nil
		},
	}
	db, _ := newMockDB(conn)
	defer db.Close()
	s := NewStore(db, nil)

	err := s.CheckAndActivateBlocked(context.Background(), uuid.New())
	if err == nil {
		t.Error("expected error from invalid uuid in scan")
	}
}

func TestCheckAndActivateBlockedInvalidDepUUID(t *testing.T) {
	// Return a row with valid goal id but invalid dep uuid in array
	id := uuid.New()
	conn := &mockConn{
		queryFunc: func(_ string, _ []driver.Value) (driver.Rows, error) {
			return &mockRows{
				cols: []string{"id", "depends_on_goal_ids"},
				data: [][]driver.Value{
					{id.String(), "{not-valid-uuid}"},
				},
			}, nil
		},
	}
	db, _ := newMockDB(conn)
	defer db.Close()
	s := NewStore(db, nil)

	err := s.CheckAndActivateBlocked(context.Background(), uuid.New())
	if err == nil {
		t.Error("expected error from invalid dep uuid")
	}
}

func TestAllDepsCompletedQueryError(t *testing.T) {
	conn := &mockConn{
		queryFunc: func(_ string, _ []driver.Value) (driver.Rows, error) {
			return nil, errGoals
		},
	}
	db, _ := newMockDB(conn)
	defer db.Close()
	s := NewStore(db, nil)

	_, err := s.allDepsCompleted(context.Background(), []uuid.UUID{uuid.New()})
	if err == nil {
		t.Error("expected error from query failure")
	}
}

func TestCheckAndActivateBlockedWithValidRowAllDepsNotDone(t *testing.T) {
	id := uuid.New()
	dep := uuid.New()

	callCount := 0
	conn := &mockConn{
		queryFunc: func(query string, args []driver.Value) (driver.Rows, error) {
			callCount++
			if callCount == 1 {
				// Main query: return one blocked goal with one dep
				return &mockRows{
					cols: []string{"id", "depends_on_goal_ids"},
					data: [][]driver.Value{
						{id.String(), "{" + dep.String() + "}"},
					},
				}, nil
			}
			// allDepsCompleted query: deps NOT all done
			return &mockRows{
				cols: []string{"?column?"},
				data: [][]driver.Value{{false}},
			}, nil
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

func TestCheckAndActivateBlockedWithValidRowAllDepsDoneActivates(t *testing.T) {
	id := uuid.New()
	dep := uuid.New()

	callCount := 0
	conn := &mockConn{
		queryFunc: func(query string, args []driver.Value) (driver.Rows, error) {
			callCount++
			if callCount == 1 {
				// Main query: one blocked goal
				return &mockRows{
					cols: []string{"id", "depends_on_goal_ids"},
					data: [][]driver.Value{
						{id.String(), "{" + dep.String() + "}"},
					},
				}, nil
			}
			// allDepsCompleted: all done
			return &mockRows{
				cols: []string{"?column?"},
				data: [][]driver.Value{{true}},
			}, nil
		},
		execFunc: func(_ string, _ []driver.Value) (driver.Result, error) {
			return driver.RowsAffected(1), nil
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
