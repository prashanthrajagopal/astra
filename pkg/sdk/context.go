package sdk

import "context"

type TaskSpec struct {
	GraphID   string
	Type      string
	Payload   []byte
	Priority  int32
	DependsOn []string
}

type Event struct {
	StreamName string
	EventType  string
	ActorID    string
	Payload    []byte
}

type ToolExecutionResult struct {
	Output            []byte
	ExitCode          int
	DurationMs        int64
	Artifacts         []string
	Status            string
	ApprovalRequestID string
}

type AgentContext interface {
	ID() string
	Memory() MemoryClient
	CreateTask(ctx context.Context, task TaskSpec) (string, error)
	PublishEvent(ctx context.Context, event Event) (string, error)
	CallTool(ctx context.Context, name string, input []byte) (ToolExecutionResult, error)
}
