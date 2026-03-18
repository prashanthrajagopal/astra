package tasks

import (
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusCreated    Status = "created"
	StatusPending    Status = "pending"
	StatusQueued     Status = "queued"
	StatusScheduled  Status = "scheduled"
	StatusRunning    Status = "running"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
	StatusDeadLetter Status = "dead_letter"
)

type Task struct {
	ID         uuid.UUID
	GraphID    uuid.UUID
	GoalID     uuid.UUID
	AgentID    uuid.UUID
	Type       string
	Status     Status
	Payload    []byte
	Result     []byte
	Priority   int
	Retries    int
	MaxRetries int
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Dependency struct {
	TaskID    uuid.UUID
	DependsOn uuid.UUID
}

type Graph struct {
	ID           uuid.UUID
	Tasks        []Task
	Dependencies []Dependency
}
