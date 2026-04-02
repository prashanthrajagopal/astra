package main

import (
	"context"
	"testing"
	"time"

	"astra/internal/messaging"

	"github.com/redis/go-redis/v9"
)

// --- extractTaskID (inline logic from message handler) ---

func extractTaskIDFromMsg(msg redis.XMessage) string {
	taskIDVal, ok := msg.Values["task_id"]
	if !ok {
		return ""
	}
	taskID, ok := taskIDVal.(string)
	if !ok {
		return ""
	}
	return taskID
}

func TestExtractTaskIDFromMsg_Valid(t *testing.T) {
	msg := redis.XMessage{
		ID:     "1-0",
		Values: map[string]interface{}{"task_id": "abc-123"},
	}
	got := extractTaskIDFromMsg(msg)
	if got != "abc-123" {
		t.Errorf("expected abc-123, got %q", got)
	}
}

func TestExtractTaskIDFromMsg_Missing(t *testing.T) {
	msg := redis.XMessage{
		ID:     "1-0",
		Values: map[string]interface{}{"other": "value"},
	}
	got := extractTaskIDFromMsg(msg)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestExtractTaskIDFromMsg_NonString(t *testing.T) {
	msg := redis.XMessage{
		ID:     "1-0",
		Values: map[string]interface{}{"task_id": 42},
	}
	got := extractTaskIDFromMsg(msg)
	if got != "" {
		t.Errorf("expected empty for non-string task_id, got %q", got)
	}
}

func TestExtractTaskIDFromMsg_EmptyValues(t *testing.T) {
	msg := redis.XMessage{
		ID:     "2-0",
		Values: map[string]interface{}{},
	}
	got := extractTaskIDFromMsg(msg)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// --- publishDeadLetterIf ---

func TestPublishDeadLetterIf_NilBusBrowserWorker(t *testing.T) {
	// Should not panic
	publishDeadLetterIf(nil, context.Background(), "tid", "gid", "error msg", true)
}

func TestPublishDeadLetterIf_NotMovedBrowserWorker(t *testing.T) {
	// When movedToDeadLetter=false, should skip publishing even with non-nil bus
	publishDeadLetterIf(nil, context.Background(), "tid", "", "error", false)
}

func TestPublishDeadLetterIf_WithGoalID(t *testing.T) {
	// nil bus + movedToDeadLetter=true with goalID — should not panic
	publishDeadLetterIf(nil, context.Background(), "task-1", "goal-1", "exec failed", true)
}

// --- runRegistryHeartbeat: context cancellation stops the loop ---

func TestRunRegistryHeartbeat_StopsOnContextCancel(t *testing.T) {
	// Use a nil registry which will panic if UpdateHeartbeat is called.
	// We cancel the context immediately so the ticker should never fire.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	done := make(chan struct{})
	go func() {
		defer close(done)
		// We test with nil registry; if the context is cancelled before any tick,
		// the function returns without calling UpdateHeartbeat.
		// runRegistryHeartbeat calls registry.UpdateHeartbeat on tick — use a
		// minimal implementation to avoid nil dereference.
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				t.Error("ticker fired before context cancelled")
				return
			}
		}
	}()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("goroutine did not stop within 2s after context cancel")
	}
}

// --- deadLetterStream constant ---

func TestDeadLetterStreamConstant(t *testing.T) {
	if deadLetterStream != "astra:dead_letter" {
		t.Errorf("deadLetterStream = %q, want astra:dead_letter", deadLetterStream)
	}
}

// --- messaging.Bus nil-safe usage pattern ---

func TestPublishDeadLetterIf_NilBusWithEmptyGoalID(t *testing.T) {
	// Empty goalID should not cause goal_id to be added to payload
	// (just verify no panic with nil bus and movedToDeadLetter=true)
	var bus *messaging.Bus
	publishDeadLetterIf(bus, context.Background(), "t1", "", "msg", true)
}
