package tasks

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		dsn = "postgres://astra:changeme@localhost:5432/astra?sslmode=disable"
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Skip("Postgres unavailable:", err)
	}
	if err := db.Ping(); err != nil {
		t.Skip("Postgres unavailable:", err)
	}
	return db
}

// TestCachedStore_NilRedis_GetTask_DelegatesToStore verifies that when Redis client is nil,
// GetTask falls through to the inner store (no cache).
func TestCachedStore_NilRedis_GetTask_DelegatesToStore(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Postgres")
	}
	db := testDB(t)
	defer db.Close()

	store := NewStore(db)
	cached := NewCachedStore(store, nil, 5*time.Minute)
	ctx := context.Background()

	task, err := cached.GetTask(ctx, "00000000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task != nil {
		t.Errorf("expected nil task for nonexistent id, got %v", task)
	}
}

// TestCachedStore_NilRedis_GetGraph_DelegatesToStore verifies that when Redis client is nil,
// GetGraph falls through to the inner store (no cache).
func TestCachedStore_NilRedis_GetGraph_DelegatesToStore(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Postgres")
	}
	db := testDB(t)
	defer db.Close()

	store := NewStore(db)
	cached := NewCachedStore(store, nil, 5*time.Minute)
	ctx := context.Background()

	graph, deps, err := cached.GetGraph(ctx, "00000000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatalf("GetGraph: %v", err)
	}
	if graph != nil || deps != nil {
		t.Errorf("expected nil graph/deps for nonexistent graph, got graph=%v deps=%v", graph, deps)
	}
}

// TestCachedStore_NilRedis_WriteMethods_Delegate verifies that with nil Redis, write methods
// (Transition, CompleteTask, etc.) delegate to store without panicking.
func TestCachedStore_NilRedis_WriteMethods_Delegate(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Postgres")
	}
	db := testDB(t)
	defer db.Close()

	store := NewStore(db)
	cached := NewCachedStore(store, nil, 5*time.Minute)
	ctx := context.Background()

	// Transition for nonexistent task should return ErrInvalidTransition
	err := cached.Transition(ctx, "00000000-0000-0000-0000-000000000001", StatusCreated, StatusPending, nil)
	if err != ErrInvalidTransition {
		t.Errorf("Transition: want ErrInvalidTransition, got %v", err)
	}

	// CompleteTask for nonexistent task should return ErrInvalidTransition
	err = cached.CompleteTask(ctx, "00000000-0000-0000-0000-000000000001", []byte("{}"))
	if err != ErrInvalidTransition {
		t.Errorf("CompleteTask: want ErrInvalidTransition, got %v", err)
	}

	// FailTask for nonexistent task should return ErrInvalidTransition
	_, err = cached.FailTask(ctx, "00000000-0000-0000-0000-000000000001", "err")
	if err != ErrInvalidTransition {
		t.Errorf("FailTask: want ErrInvalidTransition, got %v", err)
	}
}
