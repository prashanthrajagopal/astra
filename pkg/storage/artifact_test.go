package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteLocalArtifact_NoArtifactDir(t *testing.T) {
	// When ARTIFACT_DIR is unset, should return empty string with no error.
	os.Unsetenv("ARTIFACT_DIR")
	uri, err := WriteLocalArtifact("agent-1", "task-1", []byte(`{"result":"ok"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uri != "" {
		t.Errorf("expected empty uri, got %q", uri)
	}
}

func TestWriteLocalArtifact_WithArtifactDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ARTIFACT_DIR", dir)

	body := []byte(`{"answer":42}`)
	uri, err := WriteLocalArtifact("agent-1", "task-99", body)
	if err != nil {
		t.Fatalf("WriteLocalArtifact: %v", err)
	}
	if !strings.HasPrefix(uri, "file://") {
		t.Errorf("expected file:// URI, got %q", uri)
	}

	// Strip "file://" and read back
	path := strings.TrimPrefix(uri, "file://")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back file: %v", err)
	}
	if string(got) != string(body) {
		t.Errorf("content mismatch: got %q want %q", got, body)
	}
}

func TestWriteLocalArtifact_CreatesSubdirectory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ARTIFACT_DIR", dir)

	_, err := WriteLocalArtifact("my-agent", "my-task", []byte("data"))
	if err != nil {
		t.Fatalf("WriteLocalArtifact: %v", err)
	}

	subdir := filepath.Join(dir, "my-agent", "my-task")
	info, err := os.Stat(subdir)
	if err != nil {
		t.Fatalf("subdir not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}
}

func TestWriteLocalArtifact_EmptyBody(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ARTIFACT_DIR", dir)

	uri, err := WriteLocalArtifact("agent", "task", []byte{})
	if err != nil {
		t.Fatalf("WriteLocalArtifact empty body: %v", err)
	}
	if uri == "" {
		t.Error("expected non-empty URI even for empty body")
	}
	path := strings.TrimPrefix(uri, "file://")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty file, got %q", got)
	}
}

func TestWriteLocalArtifact_ResultFilename(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ARTIFACT_DIR", dir)

	uri, err := WriteLocalArtifact("agent-x", "task-y", []byte("{}"))
	if err != nil {
		t.Fatalf("WriteLocalArtifact: %v", err)
	}
	if !strings.HasSuffix(uri, "result.json") {
		t.Errorf("expected URI to end with result.json, got %q", uri)
	}
}

func TestUploadCodegenJSON_EmptyBucket(t *testing.T) {
	// When bucket is empty, should return empty string with no error (no GCS call).
	uri, err := UploadCodegenJSON(nil, "", "agent-1", "task-1", []byte("{}"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uri != "" {
		t.Errorf("expected empty uri for empty bucket, got %q", uri)
	}
}

func TestUploadCodegenJSON_WhitespaceBucket(t *testing.T) {
	// Bucket with only whitespace is treated as empty.
	uri, err := UploadCodegenJSON(nil, "   ", "agent-1", "task-1", []byte("{}"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uri != "" {
		t.Errorf("expected empty uri for whitespace bucket, got %q", uri)
	}
}
