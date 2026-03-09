package workers

import (
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

func TestRegistryRegisterAndList(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Postgres")
	}
	db := testDB(t)
	defer db.Close()

	reg := NewRegistry(db)
	ctx := t.Context()
	id := "00000000-0000-0000-0000-000000000099"

	defer db.ExecContext(ctx, "DELETE FROM workers WHERE id = $1::uuid", id)

	err := reg.Register(ctx, id, "test-host", []string{"general", "gpu"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	workers, err := reg.ListActive(ctx)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	found := false
	for _, w := range workers {
		if w.ID == id {
			found = true
			if w.Hostname != "test-host" {
				t.Errorf("hostname = %q, want test-host", w.Hostname)
			}
			if len(w.Capabilities) != 2 || w.Capabilities[0] != "general" {
				t.Errorf("capabilities = %v, want [general gpu]", w.Capabilities)
			}
		}
	}
	if !found {
		t.Error("registered worker not found in ListActive")
	}
}

func TestRegistryHeartbeatAndStale(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Postgres")
	}
	db := testDB(t)
	defer db.Close()

	reg := NewRegistry(db)
	ctx := t.Context()
	id := "00000000-0000-0000-0000-000000000098"

	defer db.ExecContext(ctx, "DELETE FROM workers WHERE id = $1::uuid", id)

	_ = reg.Register(ctx, id, "stale-host", nil)

	db.ExecContext(ctx,
		"UPDATE workers SET last_heartbeat = now() - interval '60 seconds' WHERE id = $1::uuid", id)

	stale, err := reg.FindStaleWorkers(ctx, 30*time.Second)
	if err != nil {
		t.Fatalf("FindStaleWorkers: %v", err)
	}
	found := false
	for _, sid := range stale {
		if sid == id {
			found = true
		}
	}
	if !found {
		t.Error("stale worker not found")
	}

	err = reg.MarkOffline(ctx, id)
	if err != nil {
		t.Fatalf("MarkOffline: %v", err)
	}

	active, _ := reg.ListActive(ctx)
	for _, w := range active {
		if w.ID == id {
			t.Error("offline worker still listed as active")
		}
	}
}
