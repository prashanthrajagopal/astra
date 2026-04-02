package otel

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/baggage"
)

func TestContextWithAgentID_SetsValue(t *testing.T) {
	ctx := context.Background()
	ctx = ContextWithAgentID(ctx, "agent-42")

	b := baggage.FromContext(ctx)
	m := b.Member("agent_id")
	if m.Value() != "agent-42" {
		t.Errorf("agent_id: got %q, want %q", m.Value(), "agent-42")
	}
}

func TestContextWithAgentID_EmptyID(t *testing.T) {
	ctx := context.Background()
	// Empty agentID should return the original context unchanged
	ctx2 := ContextWithAgentID(ctx, "")
	b := baggage.FromContext(ctx2)
	m := b.Member("agent_id")
	if m.Value() != "" {
		t.Errorf("expected no agent_id in baggage, got %q", m.Value())
	}
}

func TestContextWithAgentID_PreservesContext(t *testing.T) {
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "preserved")
	ctx = ContextWithAgentID(ctx, "agent-1")
	if ctx.Value(key{}) != "preserved" {
		t.Error("ContextWithAgentID must preserve existing context values")
	}
}

func TestContextWithAgentID_Table(t *testing.T) {
	tests := []struct {
		name    string
		agentID string
		wantVal string
	}{
		{"normal_id", "agent-1", "agent-1"},
		{"uuid_style", "550e8400-e29b-41d4-a716-446655440000", "550e8400-e29b-41d4-a716-446655440000"},
		{"empty_id", "", ""},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx := ContextWithAgentID(context.Background(), tc.agentID)
			b := baggage.FromContext(ctx)
			got := b.Member("agent_id").Value()
			if got != tc.wantVal {
				t.Errorf("agent_id: got %q, want %q", got, tc.wantVal)
			}
		})
	}
}

func TestContextWithAgentID_OverwritesPrevious(t *testing.T) {
	ctx := context.Background()
	ctx = ContextWithAgentID(ctx, "agent-first")
	ctx = ContextWithAgentID(ctx, "agent-second")

	b := baggage.FromContext(ctx)
	got := b.Member("agent_id").Value()
	if got != "agent-second" {
		t.Errorf("expected agent-second, got %q", got)
	}
}
