package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/google/uuid"

	"astra/internal/memory"
	"astra/pkg/config"
	"astra/pkg/db"
	"astra/pkg/grpc"
	"astra/pkg/logger"

	memorypb "astra/proto/memory"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const embeddingCacheTTL = 7 * 24 * 3600 // 7 days

type memoryServer struct {
	memorypb.UnimplementedMemoryServiceServer
	store *memory.Store
}

func (s *memoryServer) WriteMemory(ctx context.Context, req *memorypb.WriteMemoryRequest) (*memorypb.WriteMemoryResponse, error) {
	agentID, err := uuid.Parse(req.GetAgentId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid agent_id: %v", err)
	}
	memType := req.GetMemoryType()
	content := req.GetContent()
	var embedding []float32
	if len(req.GetEmbedding()) > 0 {
		embedding, err = memory.EmbeddingFromBytes(req.GetEmbedding())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "embedding: %v", err)
		}
	}
	id, err := s.store.Write(ctx, agentID, memType, content, embedding)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "write: %v", err)
	}
	return &memorypb.WriteMemoryResponse{Id: id.String()}, nil
}

func (s *memoryServer) SearchMemories(ctx context.Context, req *memorypb.SearchMemoriesRequest) (*memorypb.SearchMemoriesResponse, error) {
	agentID, err := uuid.Parse(req.GetAgentId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid agent_id: %v", err)
	}
	topK := req.GetTopK()
	if topK <= 0 {
		topK = 10
	}
	var queryEmbedding []float32
	if len(req.GetQueryEmbedding()) > 0 {
		queryEmbedding, err = memory.EmbeddingFromBytes(req.GetQueryEmbedding())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "query_embedding: %v", err)
		}
	}
	// TODO: cache-aside for read: memory:agent:{id}:search:{hash(query_embedding)}:{topK}, TTL 5min
	items, err := s.store.Search(ctx, agentID, queryEmbedding, int(topK))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "search: %v", err)
	}
	out := make([]*memorypb.MemoryItem, len(items))
	for i, m := range items {
		item := &memorypb.MemoryItem{
			Id:         m.ID.String(),
			AgentId:    m.AgentID.String(),
			MemoryType: m.MemoryType,
			Content:    m.Content,
		}
		if len(m.Embedding) > 0 {
			item.Embedding = memory.EmbeddingToBytes(m.Embedding)
		}
		out[i] = item
	}
	return &memorypb.SearchMemoriesResponse{Items: out}, nil
}

func (s *memoryServer) GetMemory(ctx context.Context, req *memorypb.GetMemoryRequest) (*memorypb.GetMemoryResponse, error) {
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid id: %v", err)
	}
	// TODO: optional cache key memory:{id}
	m, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get: %v", err)
	}
	if m == nil {
		return nil, status.Error(codes.NotFound, "memory not found")
	}
	resp := &memorypb.GetMemoryResponse{
		Id:         m.ID.String(),
		AgentId:    m.AgentID.String(),
		MemoryType: m.MemoryType,
		Content:    m.Content,
	}
	if len(m.Embedding) > 0 {
		resp.Embedding = memory.EmbeddingToBytes(m.Embedding)
	}
	return resp, nil
}

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

	var embedder memory.Embedder = memory.NewStubEmbedder()
	if cfg.MemcachedAddr != "" {
		mc := memcache.New(cfg.MemcachedAddr)
		embedder = memory.NewCachedEmbedder(embedder, mc, embeddingCacheTTL)
	}

	store := memory.NewStore(dbConn, embedder)
	srv := &memoryServer{store: store}
	grpcSrv, err := grpc.NewServerFromConfig(cfg)
	if err != nil {
		slog.Error("failed to initialize gRPC server", "err", err)
		os.Exit(1)
	}
	memorypb.RegisterMemoryServiceServer(grpcSrv, srv)

	port := cfg.MemoryGRPCPort
	if port == 0 {
		port = 9092
	}

	go func() {
		slog.Info("memory service gRPC listening", "port", port)
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
	slog.Info("memory service stopped")
}
