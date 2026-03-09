package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"astra/internal/tasks"
	"astra/pkg/config"
	"astra/pkg/db"
	"astra/pkg/grpc"
	"astra/pkg/logger"

	tasks_pb "astra/proto/tasks"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}
	slog.SetDefault(logger.New(cfg.LogLevel))

	dbConn, err := db.Connect(cfg.PostgresDSN())
	if err != nil {
		slog.Error("failed to connect to database", "err", err)
		os.Exit(1)
	}
	defer dbConn.Close()

	store := tasks.NewStore(dbConn)
	taskSrv := tasks.NewGRPCServer(store)
	srv := grpc.NewServer()
	tasks_pb.RegisterTaskServiceServer(srv, taskSrv)

	port := cfg.GRPCPort
	if port == 0 {
		port = 9090
	}

	go func() {
		slog.Info("task service gRPC listening", "port", port)
		if err := grpc.ListenAndServe(srv, port); err != nil {
			slog.Error("gRPC server error", "err", err)
			os.Exit(1)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	slog.Info("shutting down gracefully")
	srv.GracefulStop()
	slog.Info("task service stopped")
}
