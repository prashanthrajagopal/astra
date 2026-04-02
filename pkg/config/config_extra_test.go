package config

import (
	"testing"
)

// TestGetEnvBool covers all branches: empty (fallback), true values, false values, invalid.
func TestGetEnvBool(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		fallback bool
		want     bool
	}{
		{"empty uses fallback false", "", false, false},
		{"empty uses fallback true", "", true, true},
		{"true string", "true", false, true},
		{"1 string", "1", false, true},
		{"TRUE uppercase", "TRUE", false, true},
		{"false string", "false", true, false},
		{"0 string", "0", true, false},
		{"FALSE uppercase", "FALSE", true, false},
		{"invalid uses fallback false", "notabool", false, false},
		{"invalid uses fallback true", "notabool", true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key := "TEST_BOOL_" + t.Name()
			t.Setenv(key, tc.envValue)
			got := getEnvBool(key, tc.fallback)
			if got != tc.want {
				t.Errorf("getEnvBool(%q, %v) = %v, want %v", tc.envValue, tc.fallback, got, tc.want)
			}
		})
	}
}

// TestGetEnvInt covers empty (fallback), valid int, invalid (fallback).
func TestGetEnvInt(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		fallback int
		want     int
	}{
		{"empty uses fallback", "", 42, 42},
		{"valid int", "100", 0, 100},
		{"valid zero", "0", 99, 0},
		{"negative", "-5", 0, -5},
		{"invalid uses fallback", "abc", 7, 7},
		{"float is invalid", "1.5", 3, 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key := "TEST_INT_" + t.Name()
			t.Setenv(key, tc.envValue)
			got := getEnvInt(key, tc.fallback)
			if got != tc.want {
				t.Errorf("getEnvInt(%q, %d) = %d, want %d", tc.envValue, tc.fallback, got, tc.want)
			}
		})
	}
}

// TestGetEnv covers empty (fallback) and set value.
func TestGetEnv(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		fallback string
		want     string
	}{
		{"empty uses fallback", "", "default", "default"},
		{"set value returned", "custom", "default", "custom"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key := "TEST_ENV_" + t.Name()
			t.Setenv(key, tc.envValue)
			got := getEnv(key, tc.fallback)
			if got != tc.want {
				t.Errorf("getEnv(%q, %q) = %q, want %q", tc.envValue, tc.fallback, got, tc.want)
			}
		})
	}
}

// TestConfig_PostgresDSN_Components verifies each component appears correctly in DSN.
func TestConfig_PostgresDSN_Components(t *testing.T) {
	tests := []struct {
		name     string
		cfg      Config
		wantDSN  string
	}{
		{
			name: "standard",
			cfg: Config{
				PostgresHost:     "db.example.com",
				PostgresPort:     5433,
				PostgresDB:       "mydb",
				PostgresUser:     "myuser",
				PostgresPassword: "mypassword",
			},
			wantDSN: "postgres://myuser:mypassword@db.example.com:5433/mydb?sslmode=disable",
		},
		{
			name: "default values",
			cfg: Config{
				PostgresHost:     "localhost",
				PostgresPort:     5432,
				PostgresDB:       "astra",
				PostgresUser:     "astra",
				PostgresPassword: "changeme",
			},
			wantDSN: "postgres://astra:changeme@localhost:5432/astra?sslmode=disable",
		},
		{
			name: "special chars in password",
			cfg: Config{
				PostgresHost:     "localhost",
				PostgresPort:     5432,
				PostgresDB:       "db",
				PostgresUser:     "user",
				PostgresPassword: "p@ss!word",
			},
			wantDSN: "postgres://user:p@ss!word@localhost:5432/db?sslmode=disable",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.cfg.PostgresDSN()
			if got != tc.wantDSN {
				t.Errorf("PostgresDSN() = %q, want %q", got, tc.wantDSN)
			}
		})
	}
}

// TestLoad_Defaults_Extended verifies additional default values not covered by the base test.
func TestLoad_Defaults_Extended(t *testing.T) {
	// Clear all env vars that Load reads so defaults take effect.
	envKeys := []string{
		"POSTGRES_HOST", "POSTGRES_PORT", "POSTGRES_DB", "POSTGRES_USER", "POSTGRES_PASSWORD",
		"REDIS_ADDR", "MEMCACHED_ADDR", "GRPC_PORT", "AGENT_GRPC_PORT", "MEMORY_GRPC_PORT",
		"LLM_GRPC_PORT", "HTTP_PORT", "PROMPT_MANAGER_PORT", "IDENTITY_PORT",
		"ACCESS_CONTROL_PORT", "PLANNER_PORT", "GOAL_SERVICE_PORT", "EVALUATION_PORT",
		"COST_TRACKER_PORT", "IDENTITY_ADDR", "ACCESS_CONTROL_ADDR", "GOAL_SERVICE_ADDR",
		"WORKER_MANAGER_ADDR", "COST_TRACKER_ADDR", "WORKSPACE_ROOT", "LLM_GRPC_ADDR",
		"ASTRA_LOGS_DIR", "ASTRA_JWT_SECRET", "LOG_LEVEL", "OTEL_EXPORTER_OTLP_ENDPOINT",
		"ASTRA_TLS_ENABLED", "ASTRA_TLS_CERT_FILE", "ASTRA_TLS_KEY_FILE", "ASTRA_TLS_CA_FILE",
		"ASTRA_TLS_SERVER_NAME", "ASTRA_TLS_INSECURE_SKIP_VERIFY",
		"ASTRA_VAULT_ADDR", "ASTRA_VAULT_TOKEN", "ASTRA_VAULT_PATH",
		"CHAT_ENABLED", "CHAT_MAX_MSG_LENGTH", "CHAT_RATE_LIMIT", "CHAT_TOKEN_CAP",
	}
	for _, k := range envKeys {
		t.Setenv(k, "")
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	checks := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"MemcachedAddr", cfg.MemcachedAddr, "localhost:11211"},
		{"HTTPPort", cfg.HTTPPort, 8080},
		{"AgentGRPCPort", cfg.AgentGRPCPort, 9091},
		{"MemoryGRPCPort", cfg.MemoryGRPCPort, 9092},
		{"LLMGRPCPort", cfg.LLMGRPCPort, 9093},
		{"PromptManagerPort", cfg.PromptManagerPort, 8084},
		{"IdentityPort", cfg.IdentityPort, 8085},
		{"AccessControlPort", cfg.AccessControlPort, 8086},
		{"PlannerPort", cfg.PlannerPort, 8087},
		{"GoalServicePort", cfg.GoalServicePort, 8088},
		{"EvaluationPort", cfg.EvaluationPort, 8089},
		{"CostTrackerPort", cfg.CostTrackerPort, 8090},
		{"LogLevel", cfg.LogLevel, "info"},
		{"WorkspaceRoot", cfg.WorkspaceRoot, "workspace"},
		{"LogsDir", cfg.LogsDir, "logs"},
		{"JWTSecret", cfg.JWTSecret, "astra-dev-secret"},
		{"VaultPath", cfg.VaultPath, "secret/data/astra"},
		{"TLSEnabled", cfg.TLSEnabled, false},
		{"TLSInsecureSkipVerify", cfg.TLSInsecureSkipVerify, false},
		{"ChatEnabled", cfg.ChatEnabled, false},
		{"ChatMaxMsgLength", cfg.ChatMaxMsgLength, 65536},
		{"ChatRateLimit", cfg.ChatRateLimit, 30},
		{"ChatTokenCap", cfg.ChatTokenCap, 100000},
		{"IdentityAddr", cfg.IdentityAddr, "http://localhost:8085"},
		{"AccessControlAddr", cfg.AccessControlAddr, "http://localhost:8086"},
		{"GoalServiceAddr", cfg.GoalServiceAddr, "http://localhost:8088"},
		{"WorkerManagerAddr", cfg.WorkerManagerAddr, "http://localhost:8082"},
		{"CostTrackerAddr", cfg.CostTrackerAddr, "http://localhost:8090"},
		{"LLMGRPCAddr", cfg.LLMGRPCAddr, "localhost:9093"},
		{"OTELEndpoint", cfg.OTELEndpoint, "localhost:4317"},
	}

	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			if c.got != c.want {
				t.Errorf("%s: want %v, got %v", c.name, c.want, c.got)
			}
		})
	}
}

// TestLoad_EnvOverrides verifies environment variables override defaults for various field types.
func TestLoad_EnvOverrides(t *testing.T) {
	t.Setenv("ASTRA_TLS_ENABLED", "true")
	t.Setenv("ASTRA_TLS_INSECURE_SKIP_VERIFY", "true")
	t.Setenv("CHAT_ENABLED", "true")
	t.Setenv("CHAT_MAX_MSG_LENGTH", "32768")
	t.Setenv("CHAT_RATE_LIMIT", "60")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("ASTRA_VAULT_ADDR", "")   // keep empty so Vault branch not triggered
	t.Setenv("ASTRA_VAULT_TOKEN", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !cfg.TLSEnabled {
		t.Error("TLSEnabled: want true")
	}
	if !cfg.TLSInsecureSkipVerify {
		t.Error("TLSInsecureSkipVerify: want true")
	}
	if !cfg.ChatEnabled {
		t.Error("ChatEnabled: want true")
	}
	if cfg.ChatMaxMsgLength != 32768 {
		t.Errorf("ChatMaxMsgLength: want 32768, got %d", cfg.ChatMaxMsgLength)
	}
	if cfg.ChatRateLimit != 60 {
		t.Errorf("ChatRateLimit: want 60, got %d", cfg.ChatRateLimit)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel: want %q, got %q", "debug", cfg.LogLevel)
	}
}

// TestOverlayFromVault verifies overlayFromVault applies values from the map.
func TestOverlayFromVault(t *testing.T) {
	cfg := &Config{
		PostgresHost:     "old-host",
		PostgresPort:     5432,
		PostgresDB:       "old-db",
		PostgresUser:     "old-user",
		PostgresPassword: "old-pass",
		RedisAddr:        "old-redis:6379",
		JWTSecret:        "old-secret",
		TLSEnabled:       false,
		LogsDir:          "old-logs",
	}

	values := map[string]string{
		"POSTGRES_HOST":     "new-host",
		"POSTGRES_PORT":     "5433",
		"POSTGRES_DB":       "new-db",
		"POSTGRES_USER":     "new-user",
		"POSTGRES_PASSWORD": "new-pass",
		"REDIS_ADDR":        "new-redis:6380",
		"ASTRA_JWT_SECRET":  "new-secret",
		"ASTRA_TLS_ENABLED": "true",
		"ASTRA_LOGS_DIR":    "new-logs",
	}

	overlayFromVault(cfg, values)

	if cfg.PostgresHost != "new-host" {
		t.Errorf("PostgresHost: want %q, got %q", "new-host", cfg.PostgresHost)
	}
	if cfg.PostgresPort != 5433 {
		t.Errorf("PostgresPort: want 5433, got %d", cfg.PostgresPort)
	}
	if cfg.PostgresDB != "new-db" {
		t.Errorf("PostgresDB: want %q, got %q", "new-db", cfg.PostgresDB)
	}
	if cfg.PostgresUser != "new-user" {
		t.Errorf("PostgresUser: want %q, got %q", "new-user", cfg.PostgresUser)
	}
	if cfg.PostgresPassword != "new-pass" {
		t.Errorf("PostgresPassword: want %q, got %q", "new-pass", cfg.PostgresPassword)
	}
	if cfg.RedisAddr != "new-redis:6380" {
		t.Errorf("RedisAddr: want %q, got %q", "new-redis:6380", cfg.RedisAddr)
	}
	if cfg.JWTSecret != "new-secret" {
		t.Errorf("JWTSecret: want %q, got %q", "new-secret", cfg.JWTSecret)
	}
	if !cfg.TLSEnabled {
		t.Error("TLSEnabled: want true")
	}
	if cfg.LogsDir != "new-logs" {
		t.Errorf("LogsDir: want %q, got %q", "new-logs", cfg.LogsDir)
	}
}

// TestOverlayFromVault_EmptyValuesIgnored verifies that empty string values do not overwrite.
func TestOverlayFromVault_EmptyValuesIgnored(t *testing.T) {
	cfg := &Config{
		PostgresHost: "kept-host",
		JWTSecret:    "kept-secret",
	}

	// All empty — nothing should be overwritten.
	overlayFromVault(cfg, map[string]string{
		"POSTGRES_HOST":    "",
		"ASTRA_JWT_SECRET": "",
	})

	if cfg.PostgresHost != "kept-host" {
		t.Errorf("PostgresHost: want %q, got %q", "kept-host", cfg.PostgresHost)
	}
	if cfg.JWTSecret != "kept-secret" {
		t.Errorf("JWTSecret: want %q, got %q", "kept-secret", cfg.JWTSecret)
	}
}
