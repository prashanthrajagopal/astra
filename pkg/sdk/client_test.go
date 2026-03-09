package sdk

import "testing"

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.KernelGRPCAddr == "" || cfg.TaskGRPCAddr == "" || cfg.MemoryGRPCAddr == "" {
		t.Fatalf("default grpc addrs should be non-empty")
	}
	if cfg.ToolRuntimeHTTPAddr == "" {
		t.Fatalf("tool runtime addr should be non-empty")
	}
}
