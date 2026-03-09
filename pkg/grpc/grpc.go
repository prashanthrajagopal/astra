package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func loggingInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	start := time.Now()
	resp, err := handler(ctx, req)
	duration := time.Since(start)
	slog.Info("grpc", "method", info.FullMethod, "duration_ms", duration.Milliseconds(), "err", err)
	return resp, err
}

func metricsInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	// Placeholder for Prometheus metrics (latency histogram, request counter)
	return handler(ctx, req)
}

func NewServer(opts ...grpc.ServerOption) *grpc.Server {
	baseOpts := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(loggingInterceptor, metricsInterceptor),
	}
	baseOpts = append(baseOpts, opts...)
	s := grpc.NewServer(baseOpts...)
	reflection.Register(s)
	return s
}

func ListenAndServe(srv *grpc.Server, port int) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("grpc.ListenAndServe: %w", err)
	}
	return srv.Serve(lis)
}
