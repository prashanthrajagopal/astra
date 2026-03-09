package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type SandboxType string

const (
	SandboxWASM        SandboxType = "wasm"
	SandboxDocker      SandboxType = "docker"
	SandboxFirecracker SandboxType = "firecracker"
)

type ToolRequest struct {
	Name        string
	Input       []byte
	Sandbox     SandboxType
	Timeout     time.Duration
	MemoryLimit int64
	CPULimit    float64
}

type ToolResult struct {
	Output    []byte
	Artifacts []string
	ExitCode  int
	Duration  time.Duration
}

type Runtime interface {
	Execute(ctx context.Context, req ToolRequest) (ToolResult, error)
}

type DockerRuntime struct {
	Image string
}

func NewDockerRuntime() *DockerRuntime {
	return &DockerRuntime{Image: "alpine:3.20"}
}

func (r *DockerRuntime) Execute(ctx context.Context, req ToolRequest) (ToolResult, error) {
	timeout := req.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	memLimit := req.MemoryLimit
	if memLimit <= 0 {
		memLimit = 256 * 1024 * 1024
	}
	cpuLimit := req.CPULimit
	if cpuLimit <= 0 {
		cpuLimit = 1.0
	}

	image := r.Image
	if image == "" {
		image = "alpine:3.20"
	}

	args := []string{
		"run", "--rm", "-i",
		"--memory", fmt.Sprintf("%d", memLimit),
		"--cpus", fmt.Sprintf("%.1f", cpuLimit),
		"--network", "none",
		"--read-only",
		image,
		"sh", "-c", req.Name,
	}

	cmd := exec.CommandContext(execCtx, "docker", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if len(req.Input) > 0 {
		cmd.Stdin = bytes.NewReader(req.Input)
	}

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if execCtx.Err() != nil {
			return ToolResult{}, fmt.Errorf("tools.DockerRuntime: timeout after %s: %w", timeout, err)
		} else {
			return ToolResult{}, fmt.Errorf("tools.DockerRuntime: %w", err)
		}
	}

	output := stdout.Bytes()
	if stderr.Len() > 0 {
		output = append(output, []byte("\nSTDERR:\n")...)
		output = append(output, stderr.Bytes()...)
	}

	return ToolResult{
		Output:    output,
		Artifacts: []string{},
		ExitCode:  exitCode,
		Duration:  duration,
	}, nil
}

// NoopRuntime is a Phase 2 placeholder that returns success without executing.
// For browser tasks (name prefixed with "browser:"), returns placeholder HTML.
func NewNoopRuntime() *NoopRuntime {
	return &NoopRuntime{}
}

type NoopRuntime struct{}

func (r *NoopRuntime) Execute(ctx context.Context, req ToolRequest) (ToolResult, error) {
	output := []byte(`{"status":"noop","message":"Phase 2 placeholder"}`)
	if strings.HasPrefix(req.Name, "browser") {
		output = []byte(`<!DOCTYPE html><html><head><title>Placeholder</title></head><body><p>Phase 2 browser placeholder</p></body></html>`)
	}
	return ToolResult{
		Output:    output,
		Artifacts: []string{},
		ExitCode:  0,
		Duration:  0,
	}, nil
}
