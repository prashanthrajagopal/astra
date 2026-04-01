package adapters

import (
	"context"
	"encoding/json"
)

// Status represents the current state of an external job.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

// Capability describes what an external adapter can do.
type Capability struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
}

// JobResult holds the outcome of an external job.
type JobResult struct {
	Status Status          `json:"status"`
	Output json.RawMessage `json:"output,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// GoalContext carries context for dispatching a goal to an external system.
type GoalContext struct {
	GoalID   string          `json:"goal_id"`
	GoalText string          `json:"goal_text"`
	AgentID  string          `json:"agent_id"`
	Priority int             `json:"priority"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

// Adapter is the interface that all external agent adapters must implement.
type Adapter interface {
	// DispatchGoal sends a goal to the external system and returns a job ID.
	DispatchGoal(ctx context.Context, ref string, goal GoalContext) (jobID string, err error)

	// PollStatus checks the current status of a dispatched job.
	PollStatus(ctx context.Context, jobID string) (*JobResult, error)

	// HandleCallback processes an incoming callback/webhook from the external system.
	HandleCallback(ctx context.Context, payload json.RawMessage) error

	// ListCapabilities returns what this adapter can do.
	ListCapabilities(ctx context.Context) ([]Capability, error)

	// HealthCheck verifies the adapter and its external system are healthy.
	HealthCheck(ctx context.Context) (healthy bool, err error)

	// Name returns the adapter's ecosystem name (e.g., "dtec", "agentforce", "workday").
	Name() string
}
