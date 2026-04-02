package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewWorkspaceRuntime_DefaultRoot(t *testing.T) {
	w := NewWorkspaceRuntime("")
	if w == nil {
		t.Fatal("NewWorkspaceRuntime returned nil")
	}
	// Empty root defaults to "workspace" (resolved to abs path).
	if w.Root == "" {
		t.Error("Root is empty")
	}
	if !filepath.IsAbs(w.Root) {
		t.Errorf("Root %q is not absolute", w.Root)
	}
}

func TestNewWorkspaceRuntime_CustomRoot(t *testing.T) {
	dir := t.TempDir()
	w := NewWorkspaceRuntime(dir)
	if w.Root != dir {
		t.Errorf("Root = %q, want %q", w.Root, dir)
	}
}

func TestWorkspaceRuntime_SafePath_RejectsAbsolute(t *testing.T) {
	w := NewWorkspaceRuntime(t.TempDir())
	_, err := w.safePath("/etc/passwd")
	if err == nil {
		t.Error("expected error for absolute path, got nil")
	}
}

func TestWorkspaceRuntime_SafePath_RejectsTraversal(t *testing.T) {
	w := NewWorkspaceRuntime(t.TempDir())
	_, err := w.safePath("../../etc/passwd")
	if err == nil {
		t.Error("expected error for path traversal, got nil")
	}
}

func TestWorkspaceRuntime_SafePath_AcceptsRelative(t *testing.T) {
	dir := t.TempDir()
	w := NewWorkspaceRuntime(dir)
	got, err := w.safePath("subdir/file.txt")
	if err != nil {
		t.Fatalf("safePath: %v", err)
	}
	want := filepath.Join(dir, "subdir", "file.txt")
	if got != want {
		t.Errorf("safePath = %q, want %q", got, want)
	}
}

func TestWorkspaceRuntime_FileWriteRead(t *testing.T) {
	dir := t.TempDir()
	w := NewWorkspaceRuntime(dir)
	ctx := context.Background()

	writeInput, _ := json.Marshal(FileWriteRequest{
		Path:    "hello/world.txt",
		Content: "hello astra",
	})
	result, err := w.Execute(ctx, ToolRequest{Name: "file_write", Input: writeInput})
	if err != nil {
		t.Fatalf("file_write: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("file_write ExitCode = %d, want 0; output: %s", result.ExitCode, result.Output)
	}

	// Verify file was written on disk.
	data, err := os.ReadFile(filepath.Join(dir, "hello", "world.txt"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello astra" {
		t.Errorf("file content = %q, want %q", data, "hello astra")
	}

	// Read it back via file_read.
	readInput, _ := json.Marshal(map[string]string{"path": "hello/world.txt"})
	result2, err := w.Execute(ctx, ToolRequest{Name: "file_read", Input: readInput})
	if err != nil {
		t.Fatalf("file_read: %v", err)
	}
	if result2.ExitCode != 0 {
		t.Errorf("file_read ExitCode = %d, want 0; output: %s", result2.ExitCode, result2.Output)
	}
	if string(result2.Output) != "hello astra" {
		t.Errorf("file_read output = %q, want %q", result2.Output, "hello astra")
	}
}

func TestWorkspaceRuntime_FileWrite_MissingPath(t *testing.T) {
	w := NewWorkspaceRuntime(t.TempDir())
	ctx := context.Background()

	input, _ := json.Marshal(FileWriteRequest{Path: "", Content: "data"})
	result, err := w.Execute(ctx, ToolRequest{Name: "file_write", Input: input})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode == 0 {
		t.Error("expected non-zero ExitCode for missing path")
	}
}

func TestWorkspaceRuntime_FileWrite_AbsolutePath(t *testing.T) {
	w := NewWorkspaceRuntime(t.TempDir())
	ctx := context.Background()

	input, _ := json.Marshal(FileWriteRequest{Path: "/etc/evil", Content: "bad"})
	result, err := w.Execute(ctx, ToolRequest{Name: "file_write", Input: input})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode == 0 {
		t.Error("expected non-zero ExitCode for absolute path")
	}
}

func TestWorkspaceRuntime_ListFiles(t *testing.T) {
	dir := t.TempDir()
	w := NewWorkspaceRuntime(dir)
	ctx := context.Background()

	// Create some files.
	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0o644)
	_ = os.MkdirAll(filepath.Join(dir, "subdir"), 0o755)

	input, _ := json.Marshal(map[string]string{"path": "."})
	result, err := w.Execute(ctx, ToolRequest{Name: "list_files", Input: input})
	if err != nil {
		t.Fatalf("list_files: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("list_files ExitCode = %d, want 0; output: %s", result.ExitCode, result.Output)
	}

	var out map[string]any
	if err := json.Unmarshal(result.Output, &out); err != nil {
		t.Fatalf("unmarshal list_files output: %v", err)
	}
	files, ok := out["files"]
	if !ok {
		t.Fatal("list_files output missing 'files' key")
	}
	fileList, ok := files.([]any)
	if !ok {
		t.Fatalf("files is not []any: %T", files)
	}
	if len(fileList) < 3 {
		t.Errorf("expected at least 3 entries, got %d", len(fileList))
	}
}

func TestWorkspaceRuntime_UnknownToolFallsBackToShell(t *testing.T) {
	dir := t.TempDir()
	w := NewWorkspaceRuntime(dir)
	ctx := context.Background()

	input, _ := json.Marshal(map[string]string{"command": "echo astra-test"})
	result, err := w.Execute(ctx, ToolRequest{Name: "unknown_tool", Input: input})
	if err != nil {
		t.Fatalf("unknown tool shell fallback: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0; output: %s", result.ExitCode, result.Output)
	}
}

func TestWorkspaceRuntime_FileReadMissing(t *testing.T) {
	w := NewWorkspaceRuntime(t.TempDir())
	ctx := context.Background()

	input, _ := json.Marshal(map[string]string{"path": "nonexistent.txt"})
	result, err := w.Execute(ctx, ToolRequest{Name: "file_read", Input: input})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode == 0 {
		t.Error("expected non-zero ExitCode for missing file")
	}
}
