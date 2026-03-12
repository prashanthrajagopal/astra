package config

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"astra/pkg/secrets"
)

type Config struct {
	PostgresHost          string
	PostgresPort          int
	PostgresDB            string
	PostgresUser          string
	PostgresPassword      string
	RedisAddr             string
	MemcachedAddr         string
	GRPCPort              int
	AgentGRPCPort         int
	MemoryGRPCPort        int
	LLMGRPCPort           int
	HTTPPort              int
	PromptManagerPort     int
	IdentityPort          int
	AccessControlPort     int
	PlannerPort           int
	GoalServicePort       int
	EvaluationPort        int
	CostTrackerPort       int
	IdentityAddr          string
	AccessControlAddr     string
	GoalServiceAddr       string
	WorkerManagerAddr     string
	CostTrackerAddr       string
	WorkspaceRoot         string
	LLMGRPCAddr           string
	LogsDir               string
	JWTSecret             string
	LogLevel              string
	OTELEndpoint          string
	TLSEnabled            bool
	TLSCertFile           string
	TLSKeyFile            string
	TLSCAFile             string
	TLSServerName         string
	TLSInsecureSkipVerify bool
	VaultAddr             string
	VaultToken            string
	VaultPath             string
}

func Load() (*Config, error) {
	pgPort, _ := strconv.Atoi(getEnv("POSTGRES_PORT", "5432"))
	grpcPort, _ := strconv.Atoi(getEnv("GRPC_PORT", "9090"))
	agentGrpcPort, _ := strconv.Atoi(getEnv("AGENT_GRPC_PORT", "9091"))
	memoryGrpcPort, _ := strconv.Atoi(getEnv("MEMORY_GRPC_PORT", "9092"))
	llmGrpcPort, _ := strconv.Atoi(getEnv("LLM_GRPC_PORT", "9093"))
	httpPort, _ := strconv.Atoi(getEnv("HTTP_PORT", "8080"))
	promptManagerPort, _ := strconv.Atoi(getEnv("PROMPT_MANAGER_PORT", "8084"))
	identityPort, _ := strconv.Atoi(getEnv("IDENTITY_PORT", "8085"))
	accessControlPort, _ := strconv.Atoi(getEnv("ACCESS_CONTROL_PORT", "8086"))
	plannerPort, _ := strconv.Atoi(getEnv("PLANNER_PORT", "8087"))
	goalServicePort, _ := strconv.Atoi(getEnv("GOAL_SERVICE_PORT", "8088"))
	evaluationPort, _ := strconv.Atoi(getEnv("EVALUATION_PORT", "8089"))
	costTrackerPort, _ := strconv.Atoi(getEnv("COST_TRACKER_PORT", "8090"))

	cfg := &Config{
		PostgresHost:          getEnv("POSTGRES_HOST", "localhost"),
		PostgresPort:          pgPort,
		PostgresDB:            getEnv("POSTGRES_DB", "astra"),
		PostgresUser:          getEnv("POSTGRES_USER", "astra"),
		PostgresPassword:      getEnv("POSTGRES_PASSWORD", "changeme"),
		RedisAddr:             getEnv("REDIS_ADDR", "localhost:6379"),
		MemcachedAddr:         getEnv("MEMCACHED_ADDR", "localhost:11211"),
		GRPCPort:              grpcPort,
		AgentGRPCPort:         agentGrpcPort,
		MemoryGRPCPort:        memoryGrpcPort,
		LLMGRPCPort:           llmGrpcPort,
		HTTPPort:              httpPort,
		PromptManagerPort:     promptManagerPort,
		IdentityPort:          identityPort,
		AccessControlPort:     accessControlPort,
		PlannerPort:           plannerPort,
		GoalServicePort:       goalServicePort,
		EvaluationPort:        evaluationPort,
		CostTrackerPort:       costTrackerPort,
		IdentityAddr:          getEnv("IDENTITY_ADDR", "http://localhost:8085"),
		AccessControlAddr:     getEnv("ACCESS_CONTROL_ADDR", "http://localhost:8086"),
		GoalServiceAddr:       getEnv("GOAL_SERVICE_ADDR", "http://localhost:8088"),
		WorkerManagerAddr:     getEnv("WORKER_MANAGER_ADDR", "http://localhost:8082"),
		CostTrackerAddr:       getEnv("COST_TRACKER_ADDR", "http://localhost:8090"),
		WorkspaceRoot:         getEnv("WORKSPACE_ROOT", "workspace"),
		LLMGRPCAddr:           getEnv("LLM_GRPC_ADDR", "localhost:9093"),
		LogsDir:               getEnv("ASTRA_LOGS_DIR", "logs"),
		JWTSecret:             getEnv("ASTRA_JWT_SECRET", "astra-dev-secret"),
		LogLevel:              getEnv("LOG_LEVEL", "info"),
		OTELEndpoint:          getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
		TLSEnabled:            getEnvBool("ASTRA_TLS_ENABLED", false),
		TLSCertFile:           getEnv("ASTRA_TLS_CERT_FILE", ""),
		TLSKeyFile:            getEnv("ASTRA_TLS_KEY_FILE", ""),
		TLSCAFile:             getEnv("ASTRA_TLS_CA_FILE", ""),
		TLSServerName:         getEnv("ASTRA_TLS_SERVER_NAME", ""),
		TLSInsecureSkipVerify: getEnvBool("ASTRA_TLS_INSECURE_SKIP_VERIFY", false),
		VaultAddr:             getEnv("ASTRA_VAULT_ADDR", ""),
		VaultToken:            getEnv("ASTRA_VAULT_TOKEN", ""),
		VaultPath:             getEnv("ASTRA_VAULT_PATH", "secret/data/astra"),
	}

	if cfg.VaultAddr != "" && cfg.VaultToken != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		values, err := secrets.LoadKV(ctx, cfg.VaultAddr, cfg.VaultToken, cfg.VaultPath)
		if err != nil {
			slog.Warn("vault load failed, continuing with env/default config", "err", err)
		} else {
			overlayFromVault(cfg, values)
		}
	}

	return cfg, nil
}

func (c *Config) PostgresDSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		c.PostgresUser, c.PostgresPassword, c.PostgresHost, c.PostgresPort, c.PostgresDB)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return parsed
}

func overlayFromVault(cfg *Config, values map[string]string) {
	assignString := func(key string, set func(string)) {
		if v := values[key]; v != "" {
			set(v)
		}
	}
	assignInt := func(key string, set func(int)) {
		if v := values[key]; v != "" {
			if parsed, err := strconv.Atoi(v); err == nil {
				set(parsed)
			}
		}
	}
	assignBool := func(key string, set func(bool)) {
		if v := values[key]; v != "" {
			if parsed, err := strconv.ParseBool(v); err == nil {
				set(parsed)
			}
		}
	}

	assignString("POSTGRES_HOST", func(v string) { cfg.PostgresHost = v })
	assignInt("POSTGRES_PORT", func(v int) { cfg.PostgresPort = v })
	assignString("POSTGRES_DB", func(v string) { cfg.PostgresDB = v })
	assignString("POSTGRES_USER", func(v string) { cfg.PostgresUser = v })
	assignString("POSTGRES_PASSWORD", func(v string) { cfg.PostgresPassword = v })
	assignString("REDIS_ADDR", func(v string) { cfg.RedisAddr = v })
	assignString("MEMCACHED_ADDR", func(v string) { cfg.MemcachedAddr = v })
	assignString("ASTRA_JWT_SECRET", func(v string) { cfg.JWTSecret = v })
	assignString("WORKER_MANAGER_ADDR", func(v string) { cfg.WorkerManagerAddr = v })
	assignString("COST_TRACKER_ADDR", func(v string) { cfg.CostTrackerAddr = v })
	assignString("ASTRA_LOGS_DIR", func(v string) { cfg.LogsDir = v })
	assignBool("ASTRA_TLS_ENABLED", func(v bool) { cfg.TLSEnabled = v })
	assignString("ASTRA_TLS_CERT_FILE", func(v string) { cfg.TLSCertFile = v })
	assignString("ASTRA_TLS_KEY_FILE", func(v string) { cfg.TLSKeyFile = v })
	assignString("ASTRA_TLS_CA_FILE", func(v string) { cfg.TLSCAFile = v })
	assignString("ASTRA_TLS_SERVER_NAME", func(v string) { cfg.TLSServerName = v })
	assignBool("ASTRA_TLS_INSECURE_SKIP_VERIFY", func(v bool) { cfg.TLSInsecureSkipVerify = v })
}
