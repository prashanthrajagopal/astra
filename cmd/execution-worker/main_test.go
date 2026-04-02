package main

import (
	"testing"

	"github.com/redis/go-redis/v9"
)

func TestExtractTaskID(t *testing.T) {
	tests := []struct {
		name   string
		msg    redis.XMessage
		wantID string
	}{
		{
			name: "valid task_id",
			msg: redis.XMessage{
				ID:     "1-0",
				Values: map[string]interface{}{"task_id": "abc-123"},
			},
			wantID: "abc-123",
		},
		{
			name: "missing task_id key",
			msg: redis.XMessage{
				ID:     "2-0",
				Values: map[string]interface{}{"other_key": "value"},
			},
			wantID: "",
		},
		{
			name: "empty values map",
			msg: redis.XMessage{
				ID:     "3-0",
				Values: map[string]interface{}{},
			},
			wantID: "",
		},
		{
			name: "task_id is empty string",
			msg: redis.XMessage{
				ID:     "4-0",
				Values: map[string]interface{}{"task_id": ""},
			},
			wantID: "",
		},
		{
			name: "task_id is not a string (int)",
			msg: redis.XMessage{
				ID:     "5-0",
				Values: map[string]interface{}{"task_id": 42},
			},
			wantID: "",
		},
		{
			name: "task_id is not a string (bool)",
			msg: redis.XMessage{
				ID:     "6-0",
				Values: map[string]interface{}{"task_id": true},
			},
			wantID: "",
		},
		{
			name: "task_id is nil",
			msg: redis.XMessage{
				ID:     "7-0",
				Values: map[string]interface{}{"task_id": nil},
			},
			wantID: "",
		},
		{
			name: "valid uuid task_id",
			msg: redis.XMessage{
				ID:     "8-0",
				Values: map[string]interface{}{"task_id": "550e8400-e29b-41d4-a716-446655440000"},
			},
			wantID: "550e8400-e29b-41d4-a716-446655440000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTaskID(tt.msg)
			if got != tt.wantID {
				t.Errorf("extractTaskID() = %q, want %q", got, tt.wantID)
			}
		})
	}
}

func TestGetTaskShardCount(t *testing.T) {
	tests := []struct {
		name    string
		envVal  string
		want    int
		setEnv  bool
	}{
		{
			name:   "default when env not set",
			setEnv: false,
			want:   1,
		},
		{
			name:   "valid env override",
			setEnv: true,
			envVal: "4",
			want:   4,
		},
		{
			name:   "env value of 1",
			setEnv: true,
			envVal: "1",
			want:   1,
		},
		{
			name:   "large shard count",
			setEnv: true,
			envVal: "32",
			want:   32,
		},
		{
			name:   "invalid env (non-numeric) falls back to 1",
			setEnv: true,
			envVal: "invalid",
			want:   1,
		},
		{
			name:   "zero is invalid, falls back to 1",
			setEnv: true,
			envVal: "0",
			want:   1,
		},
		{
			name:   "negative is invalid, falls back to 1",
			setEnv: true,
			envVal: "-1",
			want:   1,
		},
		{
			name:   "empty string env falls back to 1",
			setEnv: true,
			envVal: "",
			want:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv("TASK_SHARD_COUNT", tt.envVal)
			} else {
				t.Setenv("TASK_SHARD_COUNT", "")
			}
			got := getTaskShardCount()
			if got != tt.want {
				t.Errorf("getTaskShardCount() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestTaskStreamForShard(t *testing.T) {
	tests := []struct {
		shard int
		want  string
	}{
		{0, "astra:tasks:shard:0"},
		{1, "astra:tasks:shard:1"},
		{7, "astra:tasks:shard:7"},
		{31, "astra:tasks:shard:31"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := taskStreamForShard(tt.shard)
			if got != tt.want {
				t.Errorf("taskStreamForShard(%d) = %q, want %q", tt.shard, got, tt.want)
			}
		})
	}
}

func TestGetEnv(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		envVal   string
		setEnv   bool
		fallback string
		want     string
	}{
		{
			name:     "returns env value when set",
			key:      "TEST_ENV_KEY",
			envVal:   "my-value",
			setEnv:   true,
			fallback: "default",
			want:     "my-value",
		},
		{
			name:     "returns fallback when env not set",
			key:      "TEST_ENV_KEY_UNSET",
			setEnv:   false,
			fallback: "default-val",
			want:     "default-val",
		},
		{
			name:     "returns fallback when env is empty string",
			key:      "TEST_ENV_KEY_EMPTY",
			envVal:   "",
			setEnv:   true,
			fallback: "fallback",
			want:     "fallback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv(tt.key, tt.envVal)
			}
			got := getEnv(tt.key, tt.fallback)
			if got != tt.want {
				t.Errorf("getEnv(%q, %q) = %q, want %q", tt.key, tt.fallback, got, tt.want)
			}
		})
	}
}
