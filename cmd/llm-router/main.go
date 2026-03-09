package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"astra/internal/events"
	"astra/internal/llm"
	"astra/internal/messaging"
	"astra/pkg/config"
	"astra/pkg/db"
	"astra/pkg/grpc"
	"astra/pkg/logger"

	llmpb "astra/proto/llm"
)

const (
	cacheTTL        = 86400 // 24h
	usageStream     = "astra:usage"
	usageGroup      = "usage-consumer"
	usageConsumerID = "llm-router-1"
)

type llmRouterServer struct {
	llmpb.UnimplementedLLMRouterServer
	router llm.Router
	bus    *messaging.Bus
}

func (s *llmRouterServer) Complete(ctx context.Context, req *llmpb.CompletionRequest) (*llmpb.CompletionResponse, error) {
	modelHint := req.GetModelHint()
	prompt := req.GetPrompt()
	options := &llm.CompletionOptions{
		ModelHint: modelHint,
		MaxTokens: int(req.GetMaxTokens()),
	}
	content, usage, err := s.router.Complete(ctx, modelHint, prompt, options)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "complete: %v", err)
	}

	// Publish usage to Redis stream (async persistence) - no DB write on hot path
	requestID := uuid.New().String()
	agentID := ""
	taskID := ""
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if v := md.Get("x-agent-id"); len(v) > 0 {
			agentID = v[0]
		}
		if v := md.Get("x-task-id"); len(v) > 0 {
			taskID = v[0]
		}
	}

	fields := map[string]interface{}{
		"request_id":   requestID,
		"model":        usage.Model,
		"tokens_in":    usage.TokensIn,
		"tokens_out":   usage.TokensOut,
		"latency_ms":   usage.LatencyMs,
		"cost_dollars": usage.CostDollars,
		"created_at":   time.Now().UTC().Format(time.RFC3339),
	}
	if agentID != "" {
		fields["agent_id"] = agentID
	}
	if taskID != "" {
		fields["task_id"] = taskID
	}

	if s.bus != nil {
		if err := s.bus.Publish(ctx, usageStream, fields); err != nil {
			slog.Warn("publish usage to stream failed", "err", err)
		}
	}

	return &llmpb.CompletionResponse{
		Content:     content,
		TokensIn:    int32(usage.TokensIn),
		TokensOut:   int32(usage.TokensOut),
		Model:       usage.Model,
		LatencyMs:   usage.LatencyMs,
		CostDollars: usage.CostDollars,
	}, nil
}

func runUsageConsumer(ctx context.Context, bus *messaging.Bus, database *sql.DB, eventStore *events.Store) {
	handler := func(msg redis.XMessage) error {
		requestID := strVal(msg.Values, "request_id")
		model := strVal(msg.Values, "model")
		tokensIn := intVal(msg.Values, "tokens_in")
		tokensOut := intVal(msg.Values, "tokens_out")
		latencyMs := intVal(msg.Values, "latency_ms")
		costDollars := floatVal(msg.Values, "cost_dollars")
		agentID := parseUUID(strVal(msg.Values, "agent_id"))
		taskID := parseUUID(strVal(msg.Values, "task_id"))

		var agentIDArg, taskIDArg interface{}
		if agentID != uuid.Nil {
			agentIDArg = agentID
		} else {
			agentIDArg = nil
		}
		if taskID != uuid.Nil {
			taskIDArg = taskID
		} else {
			taskIDArg = nil
		}

		_, err := database.ExecContext(ctx, `
			INSERT INTO llm_usage (request_id, agent_id, task_id, model, tokens_in, tokens_out, latency_ms, cost_dollars)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			requestID, agentIDArg, taskIDArg, model, tokensIn, tokensOut, latencyMs, costDollars)
		if err != nil {
			return err
		}

		payload, _ := json.Marshal(map[string]interface{}{
			"request_id": requestID,
			"model":      model,
			"tokens_in":  tokensIn,
			"tokens_out": tokensOut,
			"latency_ms": latencyMs,
			"cost":       costDollars,
		})
		_, err = eventStore.Append(ctx, "LLMUsage", requestID, payload)
		return err
	}

	bus.Consume(ctx, usageStream, usageGroup, usageConsumerID, handler)
}

func strVal(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return ""
	}
}

func intVal(m map[string]interface{}, key string) int {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch t := v.(type) {
	case string:
		n, _ := strconv.Atoi(t)
		return n
	case int64:
		return int(t)
	case int:
		return t
	case float64:
		return int(t)
	default:
		return 0
	}
}

func floatVal(m map[string]interface{}, key string) float64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch t := v.(type) {
	case string:
		f, _ := strconv.ParseFloat(t, 64)
		return f
	case float64:
		return t
	case int64:
		return float64(t)
	case int:
		return float64(t)
	default:
		return 0
	}
}

func parseUUID(s string) uuid.UUID {
	if s == "" {
		return uuid.Nil
	}
	id, _ := uuid.Parse(s)
	return id
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}
	slog.SetDefault(logger.New(cfg.LogLevel))

	database, err := db.Connect(cfg.PostgresDSN())
	if err != nil {
		slog.Error("failed to connect to database", "err", err)
		os.Exit(1)
	}
	defer database.Close()

	bus := messaging.New(cfg.RedisAddr)
	defer bus.Close()

	eventStore := events.NewStore(database)

	mc := memcache.New(cfg.MemcachedAddr)
	router := llm.NewRouterWithCache(llm.NewEndpointBackendFromEnv(), mc, cacheTTL)
	srv := &llmRouterServer{router: router, bus: bus}
	grpcSrv, err := grpc.NewServerFromConfig(cfg)
	if err != nil {
		slog.Error("failed to initialize gRPC server", "err", err)
		os.Exit(1)
	}
	llmpb.RegisterLLMRouterServer(grpcSrv, srv)

	port := cfg.LLMGRPCPort
	if port == 0 {
		port = 9093
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go runUsageConsumer(ctx, bus, database, eventStore)

	go func() {
		slog.Info("llm router gRPC listening", "port", port)
		if err := grpc.ListenAndServe(grpcSrv, port); err != nil {
			slog.Error("gRPC server error", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down gracefully")
	grpcSrv.GracefulStop()
	slog.Info("llm router stopped")
}
