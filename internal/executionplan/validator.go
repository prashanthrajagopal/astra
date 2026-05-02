package executionplan

import (
	"encoding/json"
	"fmt"
)

// Plan is the structured output that agents return as their goal result.
type Plan struct {
	Version string          `json:"version"`
	Summary string          `json:"summary,omitempty"`
	Actions []Action        `json:"actions"`
	Meta    json.RawMessage `json:"meta,omitempty"`
}

// Action is a single dispatchable unit. Type is defined by the consuming application.
type Action struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Validate checks that raw bytes are a valid execution plan envelope.
func Validate(raw []byte) (*Plan, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("execution plan is empty")
	}
	var p Plan
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("execution plan is not valid JSON: %w", err)
	}
	if p.Version == "" {
		return nil, fmt.Errorf("execution plan missing required field: version")
	}
	if p.Actions == nil {
		return nil, fmt.Errorf("execution plan missing required field: actions")
	}
	for i, a := range p.Actions {
		if a.Type == "" {
			return nil, fmt.Errorf("action[%d] missing required field: type", i)
		}
	}
	return &p, nil
}

// IsExecutionPlan returns true if raw bytes are a valid execution plan.
func IsExecutionPlan(raw []byte) bool {
	_, err := Validate(raw)
	return err == nil
}
