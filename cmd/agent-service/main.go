package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"astra/internal/actors"
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

	"github.com/google/uuid"
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

	supervisor := actors.NewSupervisor(actors.RestartBackoff, 3, time.Minute)
	onTerminate := func(agentID string) { _ = k.Stop(agentID) }
	agentFactory := func(name string) *agent.Agent {
		return agent.New(name, k, p, taskStore, database, agent.WithSupervisor(supervisor, onTerminate))
	}

	restoreAgentsFromDB(context.Background(), database, k, p, taskStore, supervisor, onTerminate)

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

func restoreAgentsFromDB(ctx context.Context, db *sql.DB, k *kernel.Kernel, p *planner.Planner, taskStore *tasks.Store, supervisor *actors.Supervisor, onTerminate func(string)) {
	rows, err := db.QueryContext(ctx, `SELECT id, name, COALESCE(actor_type, name) FROM agents WHERE status = 'active'`)
	if err != nil {
		slog.Error("restore agents: query failed", "err", err)
		return
	}
	defer rows.Close()
	var count int
	for rows.Next() {
		var id uuid.UUID
		var name, actorType string
		if err := rows.Scan(&id, &name, &actorType); err != nil {
			slog.Warn("restore agents: scan row failed", "err", err)
			continue
		}
		nameToUse := actorType
		if nameToUse == "" {
			nameToUse = name
		}
		a := agent.NewFromExisting(id, nameToUse, k, p, taskStore, db, agent.WithSupervisor(supervisor, onTerminate))
		_ = a
		count++
		slog.Info("agent restored", "agent_id", id, "name", nameToUse)
	}
	slog.Info("agent restore complete", "count", count)
}
