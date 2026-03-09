package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	PostgresHost     string
	PostgresPort     int
	PostgresDB       string
	PostgresUser     string
	PostgresPassword string
	RedisAddr        string
	MemcachedAddr    string
	GRPCPort         int
	AgentGRPCPort    int
	MemoryGRPCPort   int
	LLMGRPCPort       int
	HTTPPort          int
	PromptManagerPort int
	LogLevel          string
	OTELEndpoint     string
}

func Load() (*Config, error) {
	pgPort, _ := strconv.Atoi(getEnv("POSTGRES_PORT", "5432"))
	grpcPort, _ := strconv.Atoi(getEnv("GRPC_PORT", "9090"))
	agentGrpcPort, _ := strconv.Atoi(getEnv("AGENT_GRPC_PORT", "9091"))
	memoryGrpcPort, _ := strconv.Atoi(getEnv("MEMORY_GRPC_PORT", "9092"))
	llmGrpcPort, _ := strconv.Atoi(getEnv("LLM_GRPC_PORT", "9093"))
	httpPort, _ := strconv.Atoi(getEnv("HTTP_PORT", "8080"))
	promptManagerPort, _ := strconv.Atoi(getEnv("PROMPT_MANAGER_PORT", "8084"))

	return &Config{
		PostgresHost:     getEnv("POSTGRES_HOST", "localhost"),
		PostgresPort:     pgPort,
		PostgresDB:       getEnv("POSTGRES_DB", "astra"),
		PostgresUser:     getEnv("POSTGRES_USER", "astra"),
		PostgresPassword: getEnv("POSTGRES_PASSWORD", "changeme"),
		RedisAddr:        getEnv("REDIS_ADDR", "localhost:6379"),
		MemcachedAddr:    getEnv("MEMCACHED_ADDR", "localhost:11211"),
		GRPCPort:         grpcPort,
		AgentGRPCPort:    agentGrpcPort,
		MemoryGRPCPort:   memoryGrpcPort,
		LLMGRPCPort:       llmGrpcPort,
		HTTPPort:          httpPort,
		PromptManagerPort: promptManagerPort,
		LogLevel:          getEnv("LOG_LEVEL", "info"),
		OTELEndpoint:     getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
	}, nil
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
