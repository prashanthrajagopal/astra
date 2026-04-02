package events

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEvent_Fields(t *testing.T) {
	payload := json.RawMessage(`{"key":"value"}`)
	e := Event{
		ID:        42,
		EventType: "task_completed",
		ActorID:   "agent-abc-123",
		Payload:   payload,
		CreatedAt: time.Now().Format(time.RFC3339),
	}

	if e.ID != 42 {
		t.Errorf("ID = %d, want 42", e.ID)
	}
	if e.EventType != "task_completed" {
		t.Errorf("EventType = %q, want task_completed", e.EventType)
	}
	if e.ActorID != "agent-abc-123" {
		t.Errorf("ActorID = %q, want agent-abc-123", e.ActorID)
	}
	if string(e.Payload) != `{"key":"value"}` {
		t.Errorf("Payload = %q, want {\"key\":\"value\"}", e.Payload)
	}
	if e.CreatedAt == "" {
		t.Error("CreatedAt is empty")
	}
}

func TestEvent_ZeroValue(t *testing.T) {
	var e Event
	if e.ID != 0 {
		t.Errorf("zero ID = %d, want 0", e.ID)
	}
	if e.EventType != "" {
		t.Errorf("zero EventType = %q, want empty", e.EventType)
	}
	if e.ActorID != "" {
		t.Errorf("zero ActorID = %q, want empty", e.ActorID)
	}
	if e.Payload != nil {
		t.Errorf("zero Payload = %v, want nil", e.Payload)
	}
}

func TestNewStore_NotNil(t *testing.T) {
	// NewStore with nil db still returns a non-nil *Store (db calls will fail at runtime).
	s := NewStore(nil)
	if s == nil {
		t.Fatal("NewStore(nil) returned nil")
	}
}

func TestEvent_PayloadIsRawJSON(t *testing.T) {
	tests := []struct {
		name    string
		payload json.RawMessage
	}{
		{"object", json.RawMessage(`{"seq":1,"status":"ok"}`)},
		{"array", json.RawMessage(`[1,2,3]`)},
		{"null", json.RawMessage(`null`)},
		{"empty object", json.RawMessage(`{}`)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := Event{Payload: tc.payload}
			if string(e.Payload) != string(tc.payload) {
				t.Errorf("Payload = %q, want %q", e.Payload, tc.payload)
			}
		})
	}
}
