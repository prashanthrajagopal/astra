package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// WorkspaceRuntime executes tools scoped to a project workspace directory.
// Supported tool names: file_write, file_read, shell_exec, list_files.
// Unknown tool names are treated as shell_exec for backward compatibility.
type WorkspaceRuntime struct {
	Root string
}

func NewWorkspaceRuntime(root string) *WorkspaceRuntime {
	if root == "" {
		root = "workspace"
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	return &WorkspaceRuntime{Root: abs}
}

func (w *WorkspaceRuntime) Execute(ctx context.Context, req ToolRequest) (ToolResult, error) {
	switch req.Name {
	case "file_write":
		return w.fileWrite(ctx, req)
	case "file_read":
		return w.fileRead(ctx, req)
	case "shell_exec":
		return w.shellExec(ctx, req)
	case "list_files":
		return w.listFiles(ctx, req)
	default:
		return w.shellExec(ctx, req)
	}
}

// FileWriteRequest is the JSON input for the file_write tool.
type FileWriteRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (w *WorkspaceRuntime) fileWrite(_ context.Context, req ToolRequest) (ToolResult, error) {
	var input FileWriteRequest
	if err := json.Unmarshal(req.Input, &input); err != nil {
		return ToolResult{Output: []byte(err.Error()), ExitCode: 1}, nil
	}
	if input.Path == "" {
		return ToolResult{Output: []byte("path is required"), ExitCode: 1}, nil
	}

	target, err := w.safePath(input.Path)
	if err != nil {
		return ToolResult{Output: []byte(err.Error()), ExitCode: 1}, nil
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return ToolResult{Output: []byte(err.Error()), ExitCode: 1}, nil
	}
	if err := os.WriteFile(target, []byte(input.Content), 0o644); err != nil {
		return ToolResult{Output: []byte(err.Error()), ExitCode: 1}, nil
	}

	out, _ := json.Marshal(map[string]string{"written": input.Path, "bytes": fmt.Sprintf("%d", len(input.Content))})
	return ToolResult{Output: out, ExitCode: 0}, nil
}

func (w *WorkspaceRuntime) fileRead(_ context.Context, req ToolRequest) (ToolResult, error) {
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(req.Input, &input); err != nil {
		return ToolResult{Output: []byte(err.Error()), ExitCode: 1}, nil
	}

	target, err := w.safePath(input.Path)
	if err != nil {
		return ToolResult{Output: []byte(err.Error()), ExitCode: 1}, nil
	}

	data, err := os.ReadFile(target)
	if err != nil {
		return ToolResult{Output: []byte(err.Error()), ExitCode: 1}, nil
	}
	return ToolResult{Output: data, ExitCode: 0}, nil
}

type shellExecInput struct {
	Command string `json:"command"`
	Cwd     string `json:"cwd"`
}

func (w *WorkspaceRuntime) shellExec(ctx context.Context, req ToolRequest) (ToolResult, error) {
	var input shellExecInput

	if err := json.Unmarshal(req.Input, &input); err != nil {
		input.Command = string(req.Input)
		if input.Command == "" {
			input.Command = req.Name
		}
	}
	if input.Command == "" {
		input.Command = req.Name
	}

	dir := w.Root
	if input.Cwd != "" {
		sub, err := w.safePath(input.Cwd)
		if err != nil {
			return ToolResult{Output: []byte(err.Error()), ExitCode: 1}, nil
		}
		dir = sub
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ToolResult{Output: []byte(err.Error()), ExitCode: 1}, nil
	}

	timeout := req.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "sh", "-c", input.Command)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "HOME="+os.Getenv("HOME"))

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if execCtx.Err() != nil {
			return ToolResult{}, fmt.Errorf("workspace shell_exec: timeout after %s: %w", timeout, err)
		} else {
			return ToolResult{}, fmt.Errorf("workspace shell_exec: %w", err)
		}
	}

	output := stdout.Bytes()
	if stderr.Len() > 0 {
		output = append(output, []byte("\nSTDERR:\n")...)
		output = append(output, stderr.Bytes()...)
	}

	return ToolResult{Output: output, Artifacts: []string{}, ExitCode: exitCode, Duration: duration}, nil
}

func (w *WorkspaceRuntime) listFiles(_ context.Context, req ToolRequest) (ToolResult, error) {
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(req.Input, &input); err != nil {
		input.Path = "."
	}
	if input.Path == "" {
		input.Path = "."
	}

	target, err := w.safePath(input.Path)
	if err != nil {
		return ToolResult{Output: []byte(err.Error()), ExitCode: 1}, nil
	}

	entries, err := os.ReadDir(target)
	if err != nil {
		return ToolResult{Output: []byte(err.Error()), ExitCode: 1}, nil
	}

	var files []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		files = append(files, name)
	}

	out, _ := json.Marshal(map[string]any{"path": input.Path, "files": files})
	return ToolResult{Output: out, ExitCode: 0}, nil
}

// safePath resolves a relative path within the workspace, preventing directory traversal.
func (w *WorkspaceRuntime) safePath(rel string) (string, error) {
	cleaned := filepath.Clean(rel)
	if filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("absolute paths not allowed: %s", rel)
	}
	joined := filepath.Join(w.Root, cleaned)
	abs, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(abs, w.Root) {
		return "", fmt.Errorf("path escapes workspace: %s", rel)
	}
	return abs, nil
}
