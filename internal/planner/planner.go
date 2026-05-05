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

// planningPromptTemplate is a generic goal decomposition prompt.
// It takes (actorType, goalText) as format args.
const planningPromptTemplate = `You are a goal decomposition planner. Break the goal into a task DAG suited for the agent.

AGENT TYPE: %s

AVAILABLE TASK TYPES — choose based on the agent type and goal:

"code_generate" — write source code files. Use for software development, algorithms, scripts.
  Fields: instructions (self-contained prompt for a code LLM), output_files (list of file paths with correct extensions)

"shell_exec" — run shell commands. Use for deployments, system operations, package installs.
  Fields: commands (list of shell command strings)

"adapter_dispatch" — delegate to an external service integration. Use when the agent is designed to call an external platform (CRM, ITSM, HR, notifications, etc.) rather than write code.
  Fields: (none — goal_text and agent_context are forwarded automatically)

"llm_response" — produce structured output with an LLM. Use for analysis, recommendations, summarization, data transformation.
  Fields: instructions (what to produce), response_format ("json", "yaml", or "text")

RULES:
- Return ONLY valid JSON. No markdown fences. No explanation.
- Choose task types that match the agent type and goal.
- For adapter_dispatch agents: use adapter_dispatch tasks.
- For coding agents: use code_generate tasks with correct language file extensions.
- Simple goals: 1–3 tasks. Complex goals: up to 12 tasks with dependencies.

Schema:
{"tasks":[{"type":"code_generate","description":"...","instructions":"...","output_files":["path/file.py"]},{"type":"shell_exec","description":"...","commands":["cmd"]},{"type":"adapter_dispatch","description":"..."},{"type":"llm_response","description":"...","instructions":"...","response_format":"json"}],"dependencies":[{"task_index":1,"depends_on_index":0}]}

Goal: %s

JSON:`

type llmDAG struct {
	Tasks        []llmTask       `json:"tasks"`
	Dependencies []llmDependency `json:"dependencies"`
}

type llmTask struct {
	Type           string   `json:"type"`
	Description    string   `json:"description"`
	Instructions   string   `json:"instructions,omitempty"`
	OutputFiles    []string `json:"output_files,omitempty"`
	Commands       []string `json:"commands,omitempty"`
	ResponseFormat string   `json:"response_format,omitempty"`
}

type llmDependency struct {
	TaskIndex      int `json:"task_index"`
	DependsOnIndex int `json:"depends_on_index"`
}

// PlanOptions holds optional parameters for planning.
type PlanOptions struct {
	Workspace       string
	AgentContext    json.RawMessage
	ActorType       string // agent's actor_type, included in planning context
	DefaultTaskType string // task type to use in fallback and fast-path (e.g. "adapter_dispatch", "code_generate")
}

// Planner produces task graphs from goals.
type Planner struct {
	router llm.Router
}

func New() *Planner {
	return &Planner{router: nil}
}

func NewWithRouter(router llm.Router) *Planner {
	return &Planner{router: router}
}

// Plan produces a task graph for the goal.
//
// Fast path: when opts.DefaultTaskType == "adapter_dispatch", skips the LLM entirely
// and returns a single adapter_dispatch task. This avoids unnecessary LLM calls for
// agents whose execution is always delegated to an external adapter.
//
// Normal path: calls the LLM with a generic prompt that includes the agent's actor_type,
// allowing the LLM to choose the appropriate task types. Falls back to a deterministic
// single-task graph if the LLM call fails or the response cannot be parsed.
func (p *Planner) Plan(ctx context.Context, goalID uuid.UUID, goalText string, agentID uuid.UUID, opts *PlanOptions) (tasks.Graph, error) {
	graphID := uuid.New()
	workspace := ""
	if opts != nil {
		workspace = opts.Workspace
	}

	// Fast path: adapter agents skip the LLM entirely.
	if opts != nil && opts.DefaultTaskType == "adapter_dispatch" {
		return adapterDispatchGraph(graphID, goalID, agentID, goalText, opts), nil
	}

	if p.router != nil && goalText != "" {
		actorType := "general"
		if opts != nil && opts.ActorType != "" {
			actorType = opts.ActorType
		}
		prompt := fmt.Sprintf(planningPromptTemplate, actorType, goalText)
		if opts != nil && len(opts.AgentContext) > 0 {
			var ac struct {
				SystemPrompt string `json:"system_prompt"`
			}
			if json.Unmarshal(opts.AgentContext, &ac) == nil && ac.SystemPrompt != "" {
				prompt = "AGENT SYSTEM PROMPT: " + ac.SystemPrompt + "\n\n" + prompt
			}
		}
		resp, _, err := p.router.Complete(ctx, "code", prompt, &llm.CompletionOptions{MaxTokens: 4096})
		if err == nil {
			graph, ok := parseLLMResponse(resp, graphID, goalID, agentID, goalText, workspace, opts)
			if ok {
				slog.Info("planner: LLM produced graph", "goal_id", goalID, "task_count", len(graph.Tasks))
				return graph, nil
			}
			slog.Warn("planner: LLM response could not be parsed, using fallback", "goal_id", goalID)
		}
		if err != nil {
			slog.Warn("planner: LLM call failed, using fallback", "goal_id", goalID, "err", err)
		}
	}

	return fallbackGraph(graphID, goalID, agentID, goalText, workspace, opts), nil
}

func parseLLMResponse(resp string, graphID, goalID, agentID uuid.UUID, goalText, workspace string, opts *PlanOptions) (tasks.Graph, bool) {
	cleaned := extractJSON(resp)
	if cleaned == "" {
		return tasks.Graph{}, false
	}

	var dag llmDAG
	if err := json.Unmarshal([]byte(cleaned), &dag); err != nil {
		slog.Warn("planner: JSON parse failed", "err", err, "raw", cleaned[:min(len(cleaned), 200)])
		return tasks.Graph{}, false
	}
	if len(dag.Tasks) == 0 {
		return tasks.Graph{}, false
	}

	actorType := ""
	if opts != nil {
		actorType = opts.ActorType
	}

	taskList := make([]tasks.Task, len(dag.Tasks))
	for i := range dag.Tasks {
		tt := dag.Tasks[i]
		if tt.Type == "" {
			tt.Type = "code_generate"
		}

		var payloadMap map[string]any
		switch tt.Type {
		case "code_generate":
			payloadMap = map[string]any{
				"description":  tt.Description,
				"instructions": tt.Instructions,
				"output_files": tt.OutputFiles,
				"workspace":    workspace,
			}
		case "shell_exec":
			payloadMap = map[string]any{
				"description": tt.Description,
				"commands":    tt.Commands,
			}
		case "adapter_dispatch":
			payloadMap = map[string]any{
				"description":   tt.Description,
				"provider_type": actorType,
				"goal_text":     goalText,
			}
		case "llm_response":
			payloadMap = map[string]any{
				"description":     tt.Description,
				"instructions":    tt.Instructions,
				"response_format": tt.ResponseFormat,
			}
		default:
			tt.Type = "code_generate"
			payloadMap = map[string]any{
				"description":  tt.Description,
				"instructions": tt.Instructions,
				"output_files": tt.OutputFiles,
				"workspace":    workspace,
			}
		}
		if opts != nil && opts.AgentContext != nil {
			payloadMap["agent_context"] = opts.AgentContext
		}
		payload, _ := json.Marshal(payloadMap)
		taskList[i] = tasks.Task{
			ID:         uuid.New(),
			GraphID:    graphID,
			GoalID:     goalID,
			AgentID:    agentID,
			Type:       tt.Type,
			Status:     tasks.StatusCreated,
			Payload:    payload,
			Priority:   100,
			MaxRetries: 3,
		}
	}

	var deps []tasks.Dependency
	for _, d := range dag.Dependencies {
		if d.TaskIndex >= 0 && d.TaskIndex < len(taskList) &&
			d.DependsOnIndex >= 0 && d.DependsOnIndex < len(taskList) &&
			d.TaskIndex != d.DependsOnIndex {
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

// adapterDispatchGraph returns a single adapter_dispatch task without calling the LLM.
func adapterDispatchGraph(graphID, goalID, agentID uuid.UUID, goalText string, opts *PlanOptions) tasks.Graph {
	actorType := ""
	if opts != nil {
		actorType = opts.ActorType
	}
	payloadMap := map[string]any{
		"description":   "Dispatch goal to external adapter",
		"provider_type": actorType,
		"goal_text":     goalText,
	}
	if opts != nil && opts.AgentContext != nil {
		payloadMap["agent_context"] = opts.AgentContext
	}
	payload, _ := json.Marshal(payloadMap)
	taskID := uuid.New()
	return tasks.Graph{
		ID: graphID,
		Tasks: []tasks.Task{{
			ID:         taskID,
			GraphID:    graphID,
			GoalID:     goalID,
			AgentID:    agentID,
			Type:       "adapter_dispatch",
			Status:     tasks.StatusCreated,
			Payload:    payload,
			Priority:   100,
			MaxRetries: 3,
		}},
	}
}

// fallbackGraph returns a deterministic single-task graph used when the LLM is unavailable
// or its response cannot be parsed.
func fallbackGraph(graphID, goalID, agentID uuid.UUID, goalText, workspace string, opts *PlanOptions) tasks.Graph {
	taskType := "code_generate"
	if opts != nil && opts.DefaultTaskType != "" {
		taskType = opts.DefaultTaskType
	}

	var payloadMap map[string]any
	switch taskType {
	case "adapter_dispatch":
		actorType := ""
		if opts != nil {
			actorType = opts.ActorType
		}
		payloadMap = map[string]any{
			"description":   "Dispatch goal to external adapter",
			"provider_type": actorType,
			"goal_text":     goalText,
		}
	case "llm_response":
		payloadMap = map[string]any{
			"description":     "Generate response for goal",
			"instructions":    goalText,
			"response_format": "text",
		}
	default:
		payloadMap = map[string]any{
			"description":  "Implement the goal",
			"instructions": goalText,
			"output_files": []string{},
			"workspace":    workspace,
		}
	}
	if opts != nil && opts.AgentContext != nil {
		payloadMap["agent_context"] = opts.AgentContext
	}
	payload, _ := json.Marshal(payloadMap)
	taskID := uuid.New()
	return tasks.Graph{
		ID: graphID,
		Tasks: []tasks.Task{{
			ID:         taskID,
			GraphID:    graphID,
			GoalID:     goalID,
			AgentID:    agentID,
			Type:       taskType,
			Status:     tasks.StatusCreated,
			Payload:    payload,
			Priority:   100,
			MaxRetries: 3,
		}},
	}
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
		if end := strings.Index(s, "```"); end >= 0 {
			s = s[:end]
		}
	}
	s = strings.TrimSpace(s)
	start := strings.Index(s, "{")
	if start < 0 {
		return ""
	}
	s = s[start:]
	return findBalancedJSON(s)
}

// findBalancedJSON extracts a balanced JSON object from the beginning of s,
// ignoring any trailing text the LLM may have appended.
func findBalancedJSON(s string) string {
	depth := 0
	inString := false
	escaped := false
	for i, ch := range s {
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if ch == '{' || ch == '[' {
			depth++
		} else if ch == '}' || ch == ']' {
			depth--
			if depth == 0 {
				return s[:i+1]
			}
		}
	}
	return s
}
