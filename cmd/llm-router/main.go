package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/bradfitz/gomemcache/memcache"

	"astra/internal/llm"
	"astra/pkg/config"
	"astra/pkg/grpc"
	"astra/pkg/logger"

	llmpb "astra/proto/llm"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const cacheTTL = 86400 // 24h

type llmRouterServer struct {
	llmpb.UnimplementedLLMRouterServer
	router llm.Router
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
	return &llmpb.CompletionResponse{
		Content:     content,
		TokensIn:    int32(usage.TokensIn),
		TokensOut:   int32(usage.TokensOut),
		Model:       usage.Model,
		LatencyMs:   usage.LatencyMs,
		CostDollars: usage.CostDollars,
	}, nil
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}
	slog.SetDefault(logger.New(cfg.LogLevel))

	mc := memcache.New(cfg.MemcachedAddr)
	router := llm.NewRouterWithCache(&llm.StubBackend{}, mc, cacheTTL)
	srv := &llmRouterServer{router: router}
	grpcSrv := grpc.NewServer()
	llmpb.RegisterLLMRouterServer(grpcSrv, srv)

	port := cfg.LLMGRPCPort
	if port == 0 {
		port = 9093
	}

	go func() {
		slog.Info("llm router gRPC listening", "port", port)
		if err := grpc.ListenAndServe(grpcSrv, port); err != nil {
			slog.Error("gRPC server error", "err", err)
			os.Exit(1)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	slog.Info("shutting down gracefully")
	grpcSrv.GracefulStop()
	slog.Info("llm router stopped")
}
