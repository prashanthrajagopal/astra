package messaging

import (
	"testing"
	"time"
)

func TestNew_PoolSettings(t *testing.T) {
	b := New("localhost:6379")
	if b == nil {
		t.Fatal("New returned nil")
	}
	if b.client == nil {
		t.Fatal("Bus.client is nil")
	}
}

func TestNew_DifferentAddresses(t *testing.T) {
	tests := []struct {
		name string
		addr string
	}{
		{"localhost default", "localhost:6379"},
		{"custom port", "localhost:6380"},
		{"remote host", "redis.example.com:6379"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := New(tc.addr)
			if b == nil {
				t.Fatalf("New(%q) returned nil", tc.addr)
			}
		})
	}
}

func TestRetryKey_Format(t *testing.T) {
	b := New("localhost:6379")

	tests := []struct {
		stream  string
		group   string
		msgID   string
		want    string
	}{
		{
			stream: "astra:tasks:shard:0",
			group:  "workers",
			msgID:  "1234567890-0",
			want:   "astra:retry:astra:tasks:shard:0:workers:1234567890-0",
		},
		{
			stream: "astra:worker:events",
			group:  "monitor",
			msgID:  "9999-1",
			want:   "astra:retry:astra:worker:events:monitor:9999-1",
		},
		{
			stream: "s",
			group:  "g",
			msgID:  "m",
			want:   "astra:retry:s:g:m",
		},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			got := b.retryKey(tc.stream, tc.group, tc.msgID)
			if got != tc.want {
				t.Errorf("retryKey(%q, %q, %q) = %q, want %q",
					tc.stream, tc.group, tc.msgID, got, tc.want)
			}
		})
	}
}

func TestRetryKey_Prefix(t *testing.T) {
	b := New("localhost:6379")
	key := b.retryKey("stream", "group", "id")
	if len(key) < len(retryKeyPrefix) {
		t.Fatalf("key %q shorter than prefix %q", key, retryKeyPrefix)
	}
	if key[:len(retryKeyPrefix)] != retryKeyPrefix {
		t.Errorf("key %q does not start with prefix %q", key, retryKeyPrefix)
	}
}

func TestConstants(t *testing.T) {
	if defaultMaxRetries != 3 {
		t.Errorf("defaultMaxRetries = %d, want 3", defaultMaxRetries)
	}
	if defaultMinIdle != 30*time.Second {
		t.Errorf("defaultMinIdle = %v, want 30s", defaultMinIdle)
	}
	if retryKeyPrefix != "astra:retry:" {
		t.Errorf("retryKeyPrefix = %q, want %q", retryKeyPrefix, "astra:retry:")
	}
	if retryKeyTTL != time.Hour {
		t.Errorf("retryKeyTTL = %v, want 1h", retryKeyTTL)
	}
	if consumerDeadLetterStream != "astra:dead_letter" {
		t.Errorf("consumerDeadLetterStream = %q, want %q", consumerDeadLetterStream, "astra:dead_letter")
	}
}

func TestConsumeOptions_Defaults(t *testing.T) {
	// Zero value ConsumeOptions should result in default values being applied.
	var opts ConsumeOptions
	if opts.MaxRetries != 0 {
		t.Errorf("zero MaxRetries = %d, want 0 (triggers default)", opts.MaxRetries)
	}
	if opts.MinIdle != 0 {
		t.Errorf("zero MinIdle = %v, want 0 (triggers default)", opts.MinIdle)
	}
}

func TestConsumeOptions_ExplicitValues(t *testing.T) {
	opts := ConsumeOptions{
		MaxRetries: 5,
		MinIdle:    60 * time.Second,
	}
	if opts.MaxRetries != 5 {
		t.Errorf("MaxRetries = %d, want 5", opts.MaxRetries)
	}
	if opts.MinIdle != 60*time.Second {
		t.Errorf("MinIdle = %v, want 60s", opts.MinIdle)
	}
}
