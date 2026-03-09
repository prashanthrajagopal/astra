package planner

import (
	"astra/internal/tasks"

	"github.com/google/uuid"
)

type Planner struct{}

func New() *Planner {
	return &Planner{}
}

func (p *Planner) Plan(goalID uuid.UUID, goalText string) tasks.Graph {
	graphID := uuid.New()
	return tasks.Graph{
		ID: graphID,
		Tasks: []tasks.Task{
			{ID: uuid.New(), GraphID: graphID, GoalID: goalID, Type: "analyze", Status: tasks.StatusCreated, Priority: 100, MaxRetries: 5},
			{ID: uuid.New(), GraphID: graphID, GoalID: goalID, Type: "implement", Status: tasks.StatusCreated, Priority: 100, MaxRetries: 5},
		},
	}
}
