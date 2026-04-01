package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Agent struct {
	ID           uuid.UUID
	Name         string
	Status       string
	Config       []byte
	TrustScore   float64
	Tags         []string
	Metadata     json.RawMessage
	SystemPrompt string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Goal struct {
	ID               uuid.UUID
	AgentID          uuid.UUID
	GoalText         string
	Priority         int
	Status           string
	CascadeID        *uuid.UUID
	DependsOnGoalIDs []uuid.UUID
	CompletedAt      *time.Time
	SourceAgentID    *uuid.UUID
	CreatedAt        time.Time
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
