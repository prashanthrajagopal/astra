package models

import (
	"time"

	"github.com/google/uuid"
)

type Agent struct {
	ID        uuid.UUID
	Name      string
	Status    string
	Config    []byte
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Goal struct {
	ID        uuid.UUID
	AgentID   uuid.UUID
	GoalText  string
	Priority  int
	Status    string
	CreatedAt time.Time
}

type Event struct {
	ID        int64
	EventType string
	ActorID   uuid.UUID
	Payload   []byte
	CreatedAt time.Time
}

type Worker struct {
	ID            uuid.UUID
	Hostname      string
	Status        string
	Capabilities  []byte
	LastHeartbeat time.Time
	CreatedAt     time.Time
}

type Artifact struct {
	ID        uuid.UUID
	AgentID   uuid.UUID
	TaskID    uuid.UUID
	URI       string
	Metadata  []byte
	CreatedAt time.Time
}
