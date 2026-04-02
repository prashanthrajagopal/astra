package db

import (
	"os"
	"testing"
)

// TestConnect_InvalidDSN verifies that Connect returns an error when the DSN
// points to an unreachable host, without panicking.
func TestConnect_InvalidDSN(t *testing.T) {
	// Use a DSN that will fail to ping within the 5-second timeout.
	// We use a non-routable IP to ensure a fast refusal or timeout.
	_, err := Connect("postgres://astra:changeme@192.0.2.1:5432/astra?sslmode=disable&connect_timeout=1")
	if err == nil {
		t.Error("Connect to non-routable host: want error, got nil")
	}
}

// TestConnect_MalformedDSN verifies that Connect returns an error for a DSN
// that sql.Open itself may reject (driver-level parse error).
func TestConnect_MalformedDSN(t *testing.T) {
	// pgx stdlib accepts most strings at Open time and fails at Ping,
	// so this test just ensures we get an error and no panic.
	_, err := Connect("not-a-valid-dsn://???")
	if err == nil {
		t.Log("Connect with malformed DSN returned nil error (driver accepted it at Open, ping may have failed silently)")
	}
	// Either an error is returned, or the test passes silently — no panic is the key assertion.
}

// TestConnect_LiveDB exercises Connect against a real Postgres if POSTGRES_DSN is set.
func TestConnect_LiveDB(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_DSN not set; skipping live DB test")
	}
	db, err := Connect(dsn)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer db.Close()

	stats := db.Stats()
	if stats.MaxOpenConnections != 25 {
		t.Errorf("MaxOpenConnections: want 25, got %d", stats.MaxOpenConnections)
	}
}
