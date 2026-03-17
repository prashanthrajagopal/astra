package otel

import (
	"context"

	"go.opentelemetry.io/otel/baggage"
)

// ContextWithAgentID attaches agent_id to OpenTelemetry baggage for downstream propagation.
func ContextWithAgentID(ctx context.Context, agentID string) context.Context {
	if agentID == "" {
		return ctx
	}
	m, _ := baggage.NewMember("agent_id", agentID)
	b, _ := baggage.New(m)
	return baggage.ContextWithBaggage(ctx, b)
}
