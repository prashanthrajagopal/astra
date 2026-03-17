package agentpack

import (
	"testing"

	"astra/internal/goaladmission"
	"astra/internal/toolregistry"
)

// Harness tests agent-platform helpers without full Postgres (smoke).
func TestToolAllowlistAndAdmissionErrors(t *testing.T) {
	if toolregistry.ToolAllowed("x", []byte(`["x@1"]`)) != true {
		t.Fatal()
	}
	_ = goaladmission.ErrDrainMode
	_ = goaladmission.ErrTokenBudget
}
