package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"astra/internal/llm"
	"astra/internal/tasks"

	"github.com/google/uuid"
)

const planningPrompt = `Decompose this goal into a task DAG (directed acyclic graph). Return ONLY valid JSON, no markdown.

Schema: {"tasks":[{"type":"string","description":"optional"}],"dependencies":[{"task_index":0,"depends_on_index":1}]}
- tasks: list of task objects; type is required (e.g. "analyze", "implement", "research", "code").
- dependencies: task_index depends on depends_on_index (indices into tasks array).
- Keep it minimal: 2-5 tasks typically.

Goal: %s

JSON:`

// llmDAG is the expected JSON structure from the LLM.
type llmDAG struct {
	Tasks        []llmTask       `json:"tasks"`
	Dependencies []llmDependency `json:"dependencies"`
}

type llmTask struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

type llmDependency struct {
	TaskIndex      int `json:"task_index"`
	DependsOnIndex int `json:"depends_on_index"`
}

// Planner produces task graphs from goals.
type Planner struct {
	router llm.Router
}

// New returns a Planner with no LLM router (uses deterministic fallback).
func New() *Planner {
	return &Planner{router: nil}
}

// NewWithRouter returns a Planner that uses the given LLM router when available.
func NewWithRouter(router llm.Router) *Planner {
	return &Planner{router: router}
}

// Plan produces a task graph for the goal. Uses LLM when router is available and returns valid JSON;
// otherwise falls back to a deterministic 2-task graph (analyze -> implement).
func (p *Planner) Plan(ctx context.Context, goalID uuid.UUID, goalText string, agentID uuid.UUID) (tasks.Graph, error) {
	graphID := uuid.New()

	if p.router != nil && goalText != "" {
		prompt := fmt.Sprintf(planningPrompt, goalText)
		resp, _, err := p.router.Complete(ctx, "local", prompt, &llm.CompletionOptions{MaxTokens: 512})
		if err == nil {
			graph, ok := parseLLMResponse(resp, graphID, goalID, agentID)
			if ok {
				slog.Info("planner: LLM produced graph", "goal_id", goalID, "task_count", len(graph.Tasks))
				return graph, nil
			}
		}
		if err != nil {
			slog.Warn("planner: LLM fallback", "goal_id", goalID, "err", err)
		}
	}

	return fallbackGraph(graphID, goalID, agentID), nil
}

func parseLLMResponse(resp string, graphID, goalID, agentID uuid.UUID) (tasks.Graph, bool) {
	cleaned := extractJSON(resp)
	if cleaned == "" {
		return tasks.Graph{}, false
	}

	var dag llmDAG
	if err := json.Unmarshal([]byte(cleaned), &dag); err != nil {
		return tasks.Graph{}, false
	}
	if len(dag.Tasks) == 0 {
		return tasks.Graph{}, false
	}

	taskList := make([]tasks.Task, len(dag.Tasks))
	for i := range dag.Tasks {
		tt := dag.Tasks[i]
		taskType := tt.Type
		if taskType == "" {
			taskType = "task"
		}
		taskList[i] = tasks.Task{
			ID:         uuid.New(),
			GraphID:    graphID,
			GoalID:     goalID,
			AgentID:    agentID,
			Type:       taskType,
			Status:     tasks.StatusCreated,
			Priority:   100,
			MaxRetries: 5,
		}
	}

	var deps []tasks.Dependency
	for _, d := range dag.Dependencies {
		if d.TaskIndex >= 0 && d.TaskIndex < len(taskList) && d.DependsOnIndex >= 0 && d.DependsOnIndex < len(taskList) && d.TaskIndex != d.DependsOnIndex {
			deps = append(deps, tasks.Dependency{
				TaskID:    taskList[d.TaskIndex].ID,
				DependsOn: taskList[d.DependsOnIndex].ID,
			})
		}
	}

	return tasks.Graph{
		ID:           graphID,
		Tasks:        taskList,
		Dependencies: deps,
	}, true
}

func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.Index(s, "```"); idx >= 0 {
		s = s[idx:]
		if strings.HasPrefix(s, "```json") {
			s = s[7:]
		} else if strings.HasPrefix(s, "```") {
			s = s[3:]
		}
		if idx := strings.Index(s, "```"); idx >= 0 {
			s = s[:idx]
		}
	}
	return strings.TrimSpace(s)
}

func fallbackGraph(graphID, goalID, agentID uuid.UUID) tasks.Graph {
	analyzeID := uuid.New()
	implementID := uuid.New()
	return tasks.Graph{
		ID: graphID,
		Tasks: []tasks.Task{
			{ID: analyzeID, GraphID: graphID, GoalID: goalID, AgentID: agentID, Type: "analyze", Status: tasks.StatusCreated, Priority: 100, MaxRetries: 5},
			{ID: implementID, GraphID: graphID, GoalID: goalID, AgentID: agentID, Type: "implement", Status: tasks.StatusCreated, Priority: 100, MaxRetries: 5},
		},
		Dependencies: []tasks.Dependency{
			{TaskID: implementID, DependsOn: analyzeID},
		},
	}
}
