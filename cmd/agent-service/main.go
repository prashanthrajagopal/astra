package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"astra/internal/agent"
	"astra/internal/kernel"
	"astra/internal/kernelserver"
	"astra/internal/messaging"
	"astra/internal/planner"
	"astra/internal/tasks"
	"astra/pkg/config"
	"astra/pkg/db"
	"astra/pkg/grpc"
	"astra/pkg/logger"

	kernel_pb "astra/proto/kernel"
)

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

	k := kernel.New()
	p := planner.New()
	taskStore := tasks.NewStore(database)

	agentFactory := func(name string) *agent.Agent {
		return agent.New(name, k, p, taskStore, database)
	}

	kernelSrv := kernelserver.NewKernelGRPCServer(k, bus, database, agentFactory)
	grpcSrv, err := grpc.NewServerFromConfig(cfg)
	if err != nil {
		slog.Error("failed to initialize gRPC server", "err", err)
		os.Exit(1)
	}
	kernel_pb.RegisterKernelServiceServer(grpcSrv, kernelSrv)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	port := cfg.AgentGRPCPort
	slog.Info("agent service started", "grpc_port", port)

	go func() {
		if err := grpc.ListenAndServe(grpcSrv, port); err != nil {
			slog.Error("agent service listen failed", "err", err)
		}
	}()

	<-ctx.Done()
	grpcSrv.GracefulStop()
}
