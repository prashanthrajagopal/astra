package sdk

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"

	memorypb "astra/proto/memory"
)

type MemoryRecord struct {
	ID         string
	AgentID    string
	MemoryType string
	Content    string
	Embedding  []float32
}

type MemoryClient interface {
	Write(ctx context.Context, agentID, memoryType, content string, embedding []float32) (string, error)
	Search(ctx context.Context, agentID string, queryEmbedding []float32, topK int32) ([]MemoryRecord, error)
	GetByID(ctx context.Context, id string) (*MemoryRecord, error)
}

type grpcMemoryClient struct {
	client memorypb.MemoryServiceClient
}

func newMemoryClient(client memorypb.MemoryServiceClient) MemoryClient {
	return &grpcMemoryClient{client: client}
}

func (m *grpcMemoryClient) Write(ctx context.Context, agentID, memoryType, content string, embedding []float32) (string, error) {
	req := &memorypb.WriteMemoryRequest{
		AgentId:    agentID,
		MemoryType: memoryType,
		Content:    content,
		Embedding:  floatsToBytes(embedding),
	}
	resp, err := m.client.WriteMemory(ctx, req)
	if err != nil {
		return "", fmt.Errorf("sdk.Memory.Write: %w", err)
	}
	return resp.GetId(), nil
}

func (m *grpcMemoryClient) Search(ctx context.Context, agentID string, queryEmbedding []float32, topK int32) ([]MemoryRecord, error) {
	req := &memorypb.SearchMemoriesRequest{
		AgentId:        agentID,
		QueryEmbedding: floatsToBytes(queryEmbedding),
		TopK:           topK,
	}
	resp, err := m.client.SearchMemories(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("sdk.Memory.Search: %w", err)
	}
	out := make([]MemoryRecord, 0, len(resp.GetItems()))
	for _, item := range resp.GetItems() {
		out = append(out, MemoryRecord{
			ID:         item.GetId(),
			AgentID:    item.GetAgentId(),
			MemoryType: item.GetMemoryType(),
			Content:    item.GetContent(),
			Embedding:  bytesToFloats(item.GetEmbedding()),
		})
	}
	return out, nil
}

func (m *grpcMemoryClient) GetByID(ctx context.Context, id string) (*MemoryRecord, error) {
	resp, err := m.client.GetMemory(ctx, &memorypb.GetMemoryRequest{Id: id})
	if err != nil {
		return nil, fmt.Errorf("sdk.Memory.GetByID: %w", err)
	}
	return &MemoryRecord{
		ID:         resp.GetId(),
		AgentID:    resp.GetAgentId(),
		MemoryType: resp.GetMemoryType(),
		Content:    resp.GetContent(),
		Embedding:  bytesToFloats(resp.GetEmbedding()),
	}, nil
}

func floatsToBytes(v []float32) []byte {
	if len(v) == 0 {
		return nil
	}
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

func bytesToFloats(b []byte) []float32 {
	if len(b) == 0 || len(b)%4 != 0 {
		return nil
	}
	out := make([]float32, len(b)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return out
}
