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

const planningPrompt = `You are an expert software architect. Decompose this goal into a task DAG for an autonomous coding agent.

CRITICAL LANGUAGE RULE:
- Detect the programming language from the goal text. If the user says "ruby", generate Ruby files (.rb). If "python", generate Python files (.py). If "go" or "golang", generate Go files (.go). If "java", generate Java files (.java). If "rust", generate Rust files (.rs). And so on for any language.
- If no language is specified, default to Python (.py files).
- NEVER generate TypeScript/JavaScript/React unless the user explicitly asks for it.
- Match the file extensions, imports, syntax, and idioms to the requested language.

RULES:
- Return ONLY valid JSON, no markdown fences, no explanation.
- Each task must have: type, description, instructions, output_files.
- type MUST be "code_generate" for ALL tasks. Do NOT use "shell_exec". The agent writes files directly.
- instructions: detailed, self-contained prompt that a code-generation LLM can follow to produce the output files. MUST specify the exact programming language to use.
- output_files: list of file paths relative to the project root that this task produces. File extensions MUST match the language.
- dependencies: task_index depends_on depends_on_index (0-based indices into the tasks array).
- For simple goals (single algorithm, single script, single file): produce 1-3 tasks only. Do NOT over-engineer.
- For complex goals (web apps, multi-file projects): produce 5-12 tasks with config files first.
- Order tasks so that foundational work comes before dependent features.

Schema:
{"tasks":[{"type":"code_generate","description":"short summary","instructions":"detailed generation prompt specifying language","output_files":["path/to/file.ext"]}],"dependencies":[{"task_index":1,"depends_on_index":0}]}

Goal: %s

JSON:`

type llmDAG struct {
	Tasks        []llmTask       `json:"tasks"`
	Dependencies []llmDependency `json:"dependencies"`
}

type llmTask struct {
	Type         string   `json:"type"`
	Description  string   `json:"description"`
	Instructions string   `json:"instructions"`
	OutputFiles  []string `json:"output_files"`
}

type llmDependency struct {
	TaskIndex      int `json:"task_index"`
	DependsOnIndex int `json:"depends_on_index"`
}

// PlanOptions holds optional parameters for planning.
type PlanOptions struct {
	Workspace    string
	AgentContext json.RawMessage
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

// Plan produces a task graph for the goal. Uses LLM when router is available;
// otherwise falls back to a deterministic 2-task graph.
func (p *Planner) Plan(ctx context.Context, goalID uuid.UUID, goalText string, agentID uuid.UUID, opts *PlanOptions) (tasks.Graph, error) {
	graphID := uuid.New()
	workspace := ""
	if opts != nil {
		workspace = opts.Workspace
	}

	if p.router != nil && goalText != "" {
		prompt := fmt.Sprintf(planningPrompt, goalText)
		if opts != nil && len(opts.AgentContext) > 0 {
			var ac struct {
				SystemPrompt string `json:"system_prompt"`
			}
			if json.Unmarshal(opts.AgentContext, &ac) == nil && ac.SystemPrompt != "" {
				prompt = ac.SystemPrompt + "\n\n" + prompt
			}
		}
		resp, _, err := p.router.Complete(ctx, "code", prompt, &llm.CompletionOptions{MaxTokens: 4096})
		if err == nil {
			graph, ok := parseLLMResponse(resp, graphID, goalID, agentID, workspace, opts)
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

func parseLLMResponse(resp string, graphID, goalID, agentID uuid.UUID, workspace string, opts *PlanOptions) (tasks.Graph, bool) {
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

	taskList := make([]tasks.Task, len(dag.Tasks))
	for i := range dag.Tasks {
		tt := dag.Tasks[i]
		taskType := tt.Type
		if taskType == "" {
			taskType = "code_generate"
		}
		payloadMap := map[string]any{
			"description":  tt.Description,
			"instructions": tt.Instructions,
			"output_files": tt.OutputFiles,
			"workspace":    workspace,
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
			Type:       taskType,
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

func fallbackGraph(graphID, goalID, agentID uuid.UUID, goalText, workspace string, opts *PlanOptions) tasks.Graph {
	analyzePayloadMap := map[string]any{
		"description":  "Analyze requirements and plan the project structure",
		"instructions": "Analyze the following goal and produce a detailed implementation plan:\n\n" + goalText,
		"output_files": []string{},
		"workspace":    workspace,
	}
	implementPayloadMap := map[string]any{
		"description":  "Implement the project based on the analysis",
		"instructions": "Implement the project described by:\n\n" + goalText,
		"output_files": []string{},
		"workspace":    workspace,
	}
	if opts != nil && opts.AgentContext != nil {
		analyzePayloadMap["agent_context"] = opts.AgentContext
		implementPayloadMap["agent_context"] = opts.AgentContext
	}
	analyzePayload, _ := json.Marshal(analyzePayloadMap)
	implementPayload, _ := json.Marshal(implementPayloadMap)

	analyzeID := uuid.New()
	implementID := uuid.New()
	return tasks.Graph{
		ID: graphID,
		Tasks: []tasks.Task{
			{ID: analyzeID, GraphID: graphID, GoalID: goalID, AgentID: agentID, Type: "code_generate", Status: tasks.StatusCreated, Payload: analyzePayload, Priority: 100, MaxRetries: 3},
			{ID: implementID, GraphID: graphID, GoalID: goalID, AgentID: agentID, Type: "code_generate", Status: tasks.StatusCreated, Payload: implementPayload, Priority: 100, MaxRetries: 3},
		},
		Dependencies: []tasks.Dependency{
			{TaskID: implementID, DependsOn: analyzeID},
		},
	}
}
