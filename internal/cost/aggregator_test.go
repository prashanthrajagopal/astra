package cost

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestDailyByAgentModel(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Postgres")
	}
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		dsn = "postgres://astra:changeme@localhost:5432/astra?sslmode=disable"
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Skipf("db open failed: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Skipf("db unavailable: %v", err)
	}

	agg := NewAggregator(db)
	rows, err := agg.DailyByAgentModel(context.Background(), time.Now().Add(-30*24*time.Hour))
	if err != nil {
		t.Fatalf("DailyByAgentModel error: %v", err)
	}
	if rows == nil {
		t.Fatal("expected non-nil slice")
	}
}
