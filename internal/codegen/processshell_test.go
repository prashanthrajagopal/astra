package codegen

import (
	"context"
	"testing"

	"astra/internal/tools"
)

func TestProcessShellExecSuccess(t *testing.T) {
	dir := t.TempDir()
	runtime := tools.NewWorkspaceRuntime(dir)

	payload := TaskPayload{
		Instructions: "echo hello",
		Workspace:    "",
	}
	result, err := ProcessShellExec(context.Background(), payload, runtime)
	if err != nil {
		t.Fatalf("ProcessShellExec: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Error != "" {
		t.Errorf("unexpected error in result: %s", result.Error)
	}
}

func TestProcessShellExecUsesDescriptionWhenNoInstructions(t *testing.T) {
	dir := t.TempDir()
	runtime := tools.NewWorkspaceRuntime(dir)

	payload := TaskPayload{
		Description:  "echo fallback",
		Instructions: "",
		Workspace:    "",
	}
	result, err := ProcessShellExec(context.Background(), payload, runtime)
	if err != nil {
		t.Fatalf("ProcessShellExec: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestProcessShellExecNonZeroExit(t *testing.T) {
	dir := t.TempDir()
	runtime := tools.NewWorkspaceRuntime(dir)

	payload := TaskPayload{
		Instructions: "exit 1",
		Workspace:    "",
	}
	result, err := ProcessShellExec(context.Background(), payload, runtime)
	if err != nil {
		t.Fatalf("ProcessShellExec returned unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Error == "" {
		t.Error("expected non-empty error for non-zero exit")
	}
}

func TestProcessShellExecWithWorkspace(t *testing.T) {
	dir := t.TempDir()
	runtime := tools.NewWorkspaceRuntime(dir)

	payload := TaskPayload{
		Instructions: "echo hi",
		Workspace:    "myws",
	}
	result, err := ProcessShellExec(context.Background(), payload, runtime)
	if err != nil {
		t.Fatalf("ProcessShellExec with workspace: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}
