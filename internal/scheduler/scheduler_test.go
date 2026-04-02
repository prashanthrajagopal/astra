package scheduler

import (
	"strconv"
	"testing"
)

func TestGetTaskShardCount_DefaultIsOne(t *testing.T) {
	t.Setenv("TASK_SHARD_COUNT", "")
	got := getTaskShardCount()
	if got != 1 {
		t.Errorf("expected 1, got %d", got)
	}
}

func TestGetTaskShardCount_ReadsFromEnv(t *testing.T) {
	tests := []struct {
		envVal string
		want   int
	}{
		{"4", 4},
		{"16", 16},
		{"1", 1},
		{"100", 100},
	}
	for _, tc := range tests {
		t.Run("TASK_SHARD_COUNT="+tc.envVal, func(t *testing.T) {
			t.Setenv("TASK_SHARD_COUNT", tc.envVal)
			got := getTaskShardCount()
			if got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestGetTaskShardCount_InvalidEnvFallsBackToDefault(t *testing.T) {
	tests := []struct {
		name   string
		envVal string
	}{
		{"non-numeric", "abc"},
		{"zero", "0"},
		{"negative", "-1"},
		{"float", "1.5"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TASK_SHARD_COUNT", tc.envVal)
			got := getTaskShardCount()
			if got != 1 {
				t.Errorf("expected default 1 for env %q, got %d", tc.envVal, got)
			}
		})
	}
}

func TestShardForAgent_ReturnsZeroWhenCountLeOne(t *testing.T) {
	tests := []struct {
		count int
	}{
		{0},
		{1},
		{-1},
	}
	for _, tc := range tests {
		t.Run("count="+strconv.Itoa(tc.count), func(t *testing.T) {
			got := shardForAgent("agent-123", tc.count)
			if got != 0 {
				t.Errorf("shardForAgent with count=%d: got %d, want 0", tc.count, got)
			}
		})
	}
}

func TestShardForAgent_DistributesAcrossShards(t *testing.T) {
	count := 8
	shardsSeen := make(map[int]bool)
	agentIDs := []string{
		"agent-1", "agent-2", "agent-3", "agent-4",
		"agent-5", "agent-6", "agent-7", "agent-8",
		"agent-9", "agent-10", "agent-11", "agent-12",
		"agent-alpha", "agent-beta", "agent-gamma", "agent-delta",
	}
	for _, id := range agentIDs {
		s := shardForAgent(id, count)
		if s < 0 || s >= count {
			t.Errorf("shard %d out of range [0, %d) for agent %q", s, count, id)
		}
		shardsSeen[s] = true
	}
	// With 16 diverse agent IDs and 8 shards, expect good distribution (at least 4 different shards)
	if len(shardsSeen) < 4 {
		t.Errorf("poor distribution: only %d of %d shards used", len(shardsSeen), count)
	}
}

func TestShardForAgent_IsDeterministic(t *testing.T) {
	agentIDs := []string{"agent-abc", "agent-xyz", "user-1", "team-99"}
	count := 8
	for _, id := range agentIDs {
		t.Run(id, func(t *testing.T) {
			first := shardForAgent(id, count)
			for i := 0; i < 10; i++ {
				got := shardForAgent(id, count)
				if got != first {
					t.Errorf("non-deterministic: got %d then %d for agent %q", first, got, id)
				}
			}
		})
	}
}

func TestShardForAgent_EmptyAgentID(t *testing.T) {
	got := shardForAgent("", 4)
	if got < 0 || got >= 4 {
		t.Errorf("shard %d out of range [0,4) for empty agent ID", got)
	}
	// Must be deterministic
	if shardForAgent("", 4) != got {
		t.Error("empty agent ID should be deterministic")
	}
}

func TestTaskStreamForShard_ReturnsCorrectName(t *testing.T) {
	tests := []struct {
		shard int
		want  string
	}{
		{0, "astra:tasks:shard:0"},
		{1, "astra:tasks:shard:1"},
		{7, "astra:tasks:shard:7"},
		{15, "astra:tasks:shard:15"},
		{100, "astra:tasks:shard:100"},
	}
	for _, tc := range tests {
		t.Run("shard="+strconv.Itoa(tc.shard), func(t *testing.T) {
			got := taskStreamForShard(tc.shard)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
