package tools

import (
	"context"
	"testing"
	"time"
)

func TestNoopRuntime(t *testing.T) {
	rt := NewNoopRuntime()
	result, err := rt.Execute(context.Background(), ToolRequest{
		Name:  "echo hello",
		Input: []byte("test"),
	})
	if err != nil {
		t.Fatalf("NoopRuntime.Execute: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", result.ExitCode)
	}
	if len(result.Output) == 0 {
		t.Error("output is empty")
	}
}

func TestNoopRuntimeBrowser(t *testing.T) {
	rt := NewNoopRuntime()
	result, err := rt.Execute(context.Background(), ToolRequest{
		Name: "browser:screenshot",
	})
	if err != nil {
		t.Fatalf("NoopRuntime browser: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", result.ExitCode)
	}
	if string(result.Output) == "" || string(result.Output)[0] != '<' {
		t.Errorf("expected HTML output for browser task, got: %s", result.Output)
	}
}

func TestDockerRuntimeExecute(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Docker daemon")
	}

	rt := NewDockerRuntime()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := rt.Execute(ctx, ToolRequest{
		Name:        "echo hello-astra",
		Timeout:     30 * time.Second,
		MemoryLimit: 64 * 1024 * 1024,
		CPULimit:    0.5,
	})
	if err != nil {
		t.Fatalf("DockerRuntime.Execute: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0; output = %s", result.ExitCode, result.Output)
	}
	if string(result.Output) == "" {
		t.Error("output is empty")
	}
}

func TestDockerRuntimeTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Docker daemon")
	}

	rt := NewDockerRuntime()
	ctx := context.Background()

	_, err := rt.Execute(ctx, ToolRequest{
		Name:        "sleep 30",
		Timeout:     2 * time.Second,
		MemoryLimit: 64 * 1024 * 1024,
		CPULimit:    0.5,
	})
	if err == nil {
		t.Error("expected timeout error, got nil")
	}
}
