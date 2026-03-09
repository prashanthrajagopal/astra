package tools

import (
	"context"
	"fmt"
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

type DockerRuntime struct{}

func NewDockerRuntime() *DockerRuntime {
	return &DockerRuntime{}
}

func (r *DockerRuntime) Execute(ctx context.Context, req ToolRequest) (ToolResult, error) {
	return ToolResult{}, fmt.Errorf("tools.DockerRuntime: not yet implemented")
}
