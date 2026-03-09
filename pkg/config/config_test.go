package config

import (
	"regexp"
	"testing"
)

func TestConfig_Load_Defaults(t *testing.T) {
	keys := []string{
		"POSTGRES_HOST", "POSTGRES_PORT", "POSTGRES_DB", "POSTGRES_USER", "POSTGRES_PASSWORD",
		"REDIS_ADDR", "MEMCACHED_ADDR", "GRPC_PORT", "AGENT_GRPC_PORT", "HTTP_PORT",
		"LOG_LEVEL", "OTEL_EXPORTER_OTLP_ENDPOINT",
	}
	for _, k := range keys {
		t.Setenv(k, "")
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PostgresHost != "localhost" {
		t.Errorf("PostgresHost: expected localhost, got %s", cfg.PostgresHost)
	}
	if cfg.PostgresPort != 5432 {
		t.Errorf("PostgresPort: expected 5432, got %d", cfg.PostgresPort)
	}
	if cfg.PostgresDB != "astra" {
		t.Errorf("PostgresDB: expected astra, got %s", cfg.PostgresDB)
	}
	if cfg.PostgresUser != "astra" {
		t.Errorf("PostgresUser: expected astra, got %s", cfg.PostgresUser)
	}
	if cfg.RedisAddr != "localhost:6379" {
		t.Errorf("RedisAddr: expected localhost:6379, got %s", cfg.RedisAddr)
	}
	if cfg.GRPCPort != 9090 {
		t.Errorf("GRPCPort: expected 9090, got %d", cfg.GRPCPort)
	}
}

func TestConfig_Load_Env(t *testing.T) {
	t.Setenv("POSTGRES_HOST", "pg.example.com")
	t.Setenv("POSTGRES_PORT", "5433")
	t.Setenv("POSTGRES_DB", "testdb")
	t.Setenv("REDIS_ADDR", "redis:6380")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PostgresHost != "pg.example.com" {
		t.Errorf("PostgresHost: expected pg.example.com, got %s", cfg.PostgresHost)
	}
	if cfg.PostgresPort != 5433 {
		t.Errorf("PostgresPort: expected 5433, got %d", cfg.PostgresPort)
	}
	if cfg.PostgresDB != "testdb" {
		t.Errorf("PostgresDB: expected testdb, got %s", cfg.PostgresDB)
	}
	if cfg.RedisAddr != "redis:6380" {
		t.Errorf("RedisAddr: expected redis:6380, got %s", cfg.RedisAddr)
	}
}

func TestConfig_PostgresDSN(t *testing.T) {
	cfg := &Config{
		PostgresHost:     "localhost",
		PostgresPort:     5432,
		PostgresDB:       "astra",
		PostgresUser:     "astra",
		PostgresPassword: "secret",
	}
	dsn := cfg.PostgresDSN()
	// postgres://user:password@host:port/db?sslmode=disable
	matched, _ := regexp.MatchString(`^postgres://[^:]+:[^@]+@[^:]+:\d+/[^?]+\?sslmode=disable$`, dsn)
	if !matched {
		t.Errorf("PostgresDSN format unexpected: %s", dsn)
	}
	if dsn != "postgres://astra:secret@localhost:5432/astra?sslmode=disable" {
		t.Errorf("PostgresDSN: expected postgres://astra:secret@localhost:5432/astra?sslmode=disable, got %s", dsn)
	}
}
