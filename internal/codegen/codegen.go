package codegen

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"astra/internal/tools"
	"astra/pkg/storage"
	llmpb "astra/proto/llm"
)

// TaskPayload is the JSON structure stored in task.Payload by the planner.
type TaskPayload struct {
	Description  string          `json:"description"`
	Instructions string          `json:"instructions"`
	OutputFiles  []string        `json:"output_files"`
	Workspace    string          `json:"workspace"`
	AgentContext json.RawMessage `json:"agent_context,omitempty"`
}

// AgentContext is the deserialized agent context from the task payload.
type AgentContext struct {
	SystemPrompt string          `json:"system_prompt"`
	Rules        []AgentDocument `json:"rules"`
	Skills       []AgentDocument `json:"skills"`
	ContextDocs  []AgentDocument `json:"context_docs"`
}

// AgentDocument is a simplified document for prompt assembly.
type AgentDocument struct {
	Name    string  `json:"name"`
	Content *string `json:"content,omitempty"`
}

// Result summarizes what the code generation step produced.
type Result struct {
	FilesWritten   []string        `json:"files_written"`
	GeneratedFiles []GeneratedFile `json:"generated_files,omitempty"` // path + content for dashboard display
	LLMModel       string          `json:"llm_model"`
	TokensIn       int             `json:"tokens_in"`
	TokensOut      int             `json:"tokens_out"`
	Error          string          `json:"error,omitempty"`
	ArtifactURI    string          `json:"artifact_uri,omitempty"`
}

// GeneratedFile holds path and content of one generated file (for UI display).
type GeneratedFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

var fileBlockRe = regexp.MustCompile("(?m)^```(?:(?:typescript|tsx|ts|javascript|jsx|js|json|css|html|md|sh|bash|text)?:)?([^\\s`]+)\\s*\\n")

// Process handles a code_generate task: reads workspace context, calls LLM, writes files.
// taskID and agentID are used for artifact URIs (GCS or local).
func Process(ctx context.Context, taskID, agentID string, payload TaskPayload, runtime *tools.WorkspaceRuntime, llmClient llmpb.LLMRouterClient) (*Result, error) {
	wsRuntime := workspaceScopedRuntime(runtime, payload.Workspace)
	workspace := wsRuntime.Root

	existingContext := gatherContext(workspace, payload.OutputFiles)

	var ac *AgentContext
	if len(payload.AgentContext) > 0 {
		_ = json.Unmarshal(payload.AgentContext, &ac)
	}
	prompt := buildPrompt(payload, existingContext, ac)

	callCtx, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()

	resp, err := llmClient.Complete(callCtx, &llmpb.CompletionRequest{
		ModelHint: "code",
		Prompt:    prompt,
		MaxTokens: 8192,
	})
	if err != nil {
		return &Result{Error: fmt.Sprintf("LLM call failed: %v", err)}, err
	}

	slog.Info("codegen: LLM responded", "model", resp.GetModel(), "tokens_in", resp.GetTokensIn(), "tokens_out", resp.GetTokensOut())

	files := parseFileBlocks(resp.GetContent())
	if len(files) == 0 && len(payload.OutputFiles) > 0 {
		files = parseLooseContent(resp.GetContent(), payload.OutputFiles)
	}

	var written []string
	var generatedFiles []GeneratedFile
	for path, content := range files {
		input, _ := json.Marshal(tools.FileWriteRequest{Path: path, Content: content})
		result, err := wsRuntime.Execute(ctx, tools.ToolRequest{
			Name:    "file_write",
			Input:   input,
			Timeout: 10 * time.Second,
		})
		if err != nil || result.ExitCode != 0 {
			slog.Warn("codegen: file_write failed", "path", path, "err", err)
			continue
		}
		written = append(written, path)
		generatedFiles = append(generatedFiles, GeneratedFile{Path: path, Content: content})
	}

	out := &Result{
		FilesWritten:   written,
		GeneratedFiles: generatedFiles,
		LLMModel:       resp.GetModel(),
		TokensIn:       int(resp.GetTokensIn()),
		TokensOut:      int(resp.GetTokensOut()),
	}
	if taskID != "" {
		summary, _ := json.Marshal(map[string]any{
			"files_written": written, "llm_model": out.LLMModel, "tokens_in": out.TokensIn, "tokens_out": out.TokensOut,
		})
		if uri, err := storage.UploadCodegenJSON(ctx, os.Getenv("GCS_BUCKET"), agentID, taskID, summary); err == nil && uri != "" {
			out.ArtifactURI = uri
		} else if uri2, err2 := storage.WriteLocalArtifact(agentID, taskID, summary); err2 == nil && uri2 != "" {
			out.ArtifactURI = uri2
		}
	}
	return out, nil
}

// ProcessShellExec handles a shell_exec task from the planner.
func ProcessShellExec(ctx context.Context, payload TaskPayload, runtime *tools.WorkspaceRuntime) (*Result, error) {
	cmd := payload.Instructions
	if cmd == "" {
		cmd = payload.Description
	}
	cmd = cleanShellCommand(cmd)

	wsRuntime := workspaceScopedRuntime(runtime, payload.Workspace)
	input, _ := json.Marshal(map[string]string{"command": cmd})
	result, err := wsRuntime.Execute(ctx, tools.ToolRequest{
		Name:    "shell_exec",
		Input:   input,
		Timeout: 180 * time.Second,
	})
	if err != nil {
		return &Result{Error: fmt.Sprintf("shell_exec failed: %v", err)}, err
	}
	if result.ExitCode != 0 {
		return &Result{Error: fmt.Sprintf("shell exited %d: %s", result.ExitCode, string(result.Output))}, nil
	}

	return &Result{FilesWritten: []string{}}, nil
}

func buildPrompt(payload TaskPayload, existingContext map[string]string, ac *AgentContext) string {
	var sb strings.Builder

	if ac != nil && ac.SystemPrompt != "" {
		sb.WriteString(ac.SystemPrompt)
		sb.WriteString("\n\n")
	}

	sb.WriteString("You are a senior full-stack developer. Generate production-quality code.\n\n")

	if ac != nil && len(ac.Rules) > 0 {
		sb.WriteString("RULES:\n")
		for _, r := range ac.Rules {
			if r.Content != nil && *r.Content != "" {
				sb.WriteString(*r.Content)
				sb.WriteString("\n")
			}
		}
		sb.WriteString("- Return ONLY code files, no explanation or commentary.\n")
	} else {
		sb.WriteString("RULES:\n")
		sb.WriteString("- Return ONLY code files, no explanation or commentary.\n")
	}
	sb.WriteString("- For each file, use this exact format:\n")
	sb.WriteString("```filepath:path/to/file.ext\n")
	sb.WriteString("// file content here\n")
	sb.WriteString("```\n")
	sb.WriteString("- Generate complete, working files. Do not use placeholders or TODOs.\n")
	lang := detectLanguageFromTask(payload)
	sb.WriteString("- Use " + lang + ".\n")
	sb.WriteString("- Match file extensions, imports, and idioms to the language.\n\n")

	if ac != nil && len(ac.Skills) > 0 {
		sb.WriteString("SKILLS:\n")
		for _, s := range ac.Skills {
			if s.Content != nil && *s.Content != "" {
				sb.WriteString(*s.Content)
				sb.WriteString("\n\n")
			}
		}
	}

	if ac != nil && len(ac.ContextDocs) > 0 {
		sb.WriteString("CONTEXT:\n")
		for _, c := range ac.ContextDocs {
			if c.Content != nil && *c.Content != "" {
				sb.WriteString(*c.Content)
				sb.WriteString("\n\n")
			}
		}
	}

	if len(existingContext) > 0 {
		sb.WriteString("EXISTING PROJECT FILES (for context, do not regenerate unless instructed):\n\n")
		for path, content := range existingContext {
			sb.WriteString(fmt.Sprintf("--- %s ---\n%s\n\n", path, truncate(content, 2000)))
		}
	}

	sb.WriteString("TASK:\n")
	sb.WriteString(payload.Instructions)
	sb.WriteString("\n\n")

	if len(payload.OutputFiles) > 0 {
		sb.WriteString("FILES TO GENERATE:\n")
		for _, f := range payload.OutputFiles {
			sb.WriteString(fmt.Sprintf("- %s\n", f))
		}
	}

	return sb.String()
}

func detectLanguageFromTask(payload TaskPayload) string {
	all := strings.ToLower(payload.Instructions + " " + strings.Join(payload.OutputFiles, " "))
	switch {
	case strings.Contains(all, ".rb") || strings.Contains(all, "ruby"):
		return "Ruby"
	case strings.Contains(all, ".py") || strings.Contains(all, "python"):
		return "Python"
	case strings.Contains(all, ".go") || strings.Contains(all, "golang"):
		return "Go"
	case strings.Contains(all, ".rs") || strings.Contains(all, "rust"):
		return "Rust"
	case strings.Contains(all, ".java") || strings.Contains(all, "java "):
		return "Java"
	case strings.Contains(all, ".cs") || strings.Contains(all, "c#") || strings.Contains(all, "csharp"):
		return "C#"
	case strings.Contains(all, ".cpp") || strings.Contains(all, ".cc") || strings.Contains(all, "c++"):
		return "C++"
	case strings.Contains(all, ".c ") || strings.Contains(all, ".h "):
		return "C"
	case strings.Contains(all, ".php") || strings.Contains(all, "php"):
		return "PHP"
	case strings.Contains(all, ".swift") || strings.Contains(all, "swift"):
		return "Swift"
	case strings.Contains(all, ".kt") || strings.Contains(all, "kotlin"):
		return "Kotlin"
	case strings.Contains(all, ".scala") || strings.Contains(all, "scala"):
		return "Scala"
	case strings.Contains(all, ".tsx") || strings.Contains(all, ".ts") || strings.Contains(all, "typescript"):
		return "TypeScript with React and Next.js"
	case strings.Contains(all, ".jsx") || strings.Contains(all, ".js") || strings.Contains(all, "javascript"):
		return "JavaScript"
	case strings.Contains(all, ".sh") || strings.Contains(all, "bash") || strings.Contains(all, "shell"):
		return "Bash/Shell"
	default:
		return "Python"
	}
}

func gatherContext(workspace string, outputFiles []string) map[string]string {
	context := make(map[string]string)

	contextFiles := []string{
		"package.json",
		"tsconfig.json",
		"tailwind.config.ts",
		"src/app/layout.tsx",
		"src/types/index.ts",
		"src/data/products.ts",
		"src/context/CartContext.tsx",
	}

	for _, rel := range contextFiles {
		full := filepath.Join(workspace, rel)
		data, err := os.ReadFile(full)
		if err == nil && len(data) > 0 {
			context[rel] = string(data)
		}
	}

	for _, rel := range outputFiles {
		full := filepath.Join(workspace, rel)
		data, err := os.ReadFile(full)
		if err == nil && len(data) > 0 {
			context[rel] = string(data)
		}
	}
	return context
}

// parseFileBlocks extracts file path/content pairs from LLM output using ```filepath:path format.
func parseFileBlocks(content string) map[string]string {
	files := make(map[string]string)
	locs := fileBlockRe.FindAllStringSubmatchIndex(content, -1)
	for i, loc := range locs {
		path := content[loc[2]:loc[3]]
		start := loc[1]
		end := findBlockEnd(content, start, locs, i)
		body := strings.TrimRight(content[start:end], " \t\n")
		if path != "" && body != "" {
			files[path] = body
		}
	}
	if len(files) == 0 {
		files = parseFallbackBlocks(content)
	}
	return files
}

func findBlockEnd(content string, start int, locs [][]int, i int) int {
	if i+1 < len(locs) {
		searchArea := content[start:locs[i+1][0]]
		if idx := strings.Index(searchArea, "```"); idx >= 0 {
			return start + idx
		}
		return locs[i+1][0]
	}
	remaining := content[start:]
	if idx := strings.Index(remaining, "```"); idx >= 0 {
		return start + idx
	}
	return len(content)
}

// parseFallbackBlocks handles the common pattern: ```lang\n// filename: path\n...```
func parseFallbackBlocks(content string) map[string]string {
	files := make(map[string]string)
	parts := strings.Split(content, "```")
	for i := 1; i < len(parts); i += 2 {
		path, body := extractPathFromBlock(parts[i])
		if path != "" {
			files[path] = body
		}
	}
	return files
}

func extractPathFromBlock(block string) (string, string) {
	lines := strings.SplitN(block, "\n", 2)
	if len(lines) < 2 {
		return "", ""
	}
	bodyLines := strings.SplitN(lines[1], "\n", 2)
	if len(bodyLines) < 2 {
		return "", ""
	}
	firstLine := strings.TrimSpace(bodyLines[0])
	candidate := strings.TrimPrefix(firstLine, "// ")
	candidate = strings.TrimPrefix(candidate, "/* ")
	candidate = strings.TrimSuffix(candidate, " */")
	candidate = strings.TrimSpace(candidate)
	if strings.Contains(candidate, "/") && !strings.Contains(candidate, " ") {
		return candidate, lines[1]
	}
	return "", ""
}

// parseLooseContent is a last resort: if no file blocks found and only one output file expected,
// treat the entire LLM response as the file content.
func parseLooseContent(content string, outputFiles []string) map[string]string {
	if len(outputFiles) != 1 {
		return nil
	}
	cleaned := strings.TrimSpace(content)
	if strings.HasPrefix(cleaned, "```") {
		lines := strings.SplitN(cleaned, "\n", 2)
		if len(lines) > 1 {
			cleaned = lines[1]
		}
		if idx := strings.LastIndex(cleaned, "```"); idx >= 0 {
			cleaned = cleaned[:idx]
		}
	}
	if cleaned == "" {
		return nil
	}
	return map[string]string{outputFiles[0]: cleaned}
}

// workspaceScopedRuntime returns a WorkspaceRuntime under the base root (single-platform: no org).
func workspaceScopedRuntime(runtime *tools.WorkspaceRuntime, workspace string) *tools.WorkspaceRuntime {
	scopedRoot := filepath.Join(runtime.Root, "_global")
	if workspace == "" {
		return tools.NewWorkspaceRuntime(scopedRoot)
	}
	if filepath.IsAbs(workspace) {
		return tools.NewWorkspaceRuntime(workspace)
	}
	return tools.NewWorkspaceRuntime(filepath.Join(scopedRoot, workspace))
}

// cleanShellCommand strips common LLM formatting from shell commands.
func cleanShellCommand(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	for _, prefix := range []string{"Run:", "run:", "Execute:", "execute:", "Command:", "command:", "$ "} {
		cmd = strings.TrimPrefix(cmd, prefix)
	}
	cmd = strings.TrimSpace(cmd)
	if idx := strings.Index(cmd, "\n"); idx >= 0 {
		cmd = cmd[:idx]
	}
	return cmd
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "\n... (truncated)"
}
