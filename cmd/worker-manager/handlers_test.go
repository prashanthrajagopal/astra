package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleHealth(t *testing.T) {
	srv := &server{}
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.handleHealth(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Errorf("expected body 'ok', got %q", w.Body.String())
	}
}

func TestShardForAgent(t *testing.T) {
	tests := []struct {
		agentID string
		count   int
		wantGE  int
		wantLT  int
	}{
		{"agent-abc", 4, 0, 4},
		{"agent-xyz", 4, 0, 4},
		{"agent-abc", 1, 0, 1},
		{"", 4, 0, 4},
	}
	for _, tt := range tests {
		got := shardForAgent(tt.agentID, tt.count)
		if got < tt.wantGE || got >= tt.wantLT {
			t.Errorf("shardForAgent(%q, %d) = %d, want [%d, %d)", tt.agentID, tt.count, got, tt.wantGE, tt.wantLT)
		}
	}
}

func TestShardForAgentCount1(t *testing.T) {
	// With count=1, always shard 0.
	if got := shardForAgent("any-agent", 1); got != 0 {
		t.Errorf("shardForAgent count=1 = %d, want 0", got)
	}
}

func TestShardForAgentDeterministic(t *testing.T) {
	// Same input should always produce same shard.
	a := shardForAgent("stable-agent-id", 8)
	b := shardForAgent("stable-agent-id", 8)
	if a != b {
		t.Errorf("shardForAgent is not deterministic: %d != %d", a, b)
	}
}

func TestTaskStreamForShard(t *testing.T) {
	if got := taskStreamForShard(0); got != "astra:tasks:shard:0" {
		t.Errorf("unexpected stream: %q", got)
	}
	if got := taskStreamForShard(3); got != "astra:tasks:shard:3" {
		t.Errorf("unexpected stream: %q", got)
	}
}

func TestGetTaskShardCount_Default(t *testing.T) {
	t.Setenv("TASK_SHARD_COUNT", "")
	if got := getTaskShardCount(); got != 1 {
		t.Errorf("expected default 1, got %d", got)
	}
}

func TestGetTaskShardCount_Env(t *testing.T) {
	t.Setenv("TASK_SHARD_COUNT", "5")
	if got := getTaskShardCount(); got != 5 {
		t.Errorf("expected 5, got %d", got)
	}
}

func TestGetTaskShardCount_Invalid(t *testing.T) {
	t.Setenv("TASK_SHARD_COUNT", "notanumber")
	if got := getTaskShardCount(); got != 1 {
		t.Errorf("expected fallback 1, got %d", got)
	}
}

func TestGetTaskShardCount_Zero(t *testing.T) {
	t.Setenv("TASK_SHARD_COUNT", "0")
	if got := getTaskShardCount(); got != 1 {
		t.Errorf("expected fallback 1 for zero, got %d", got)
	}
}
