package sdk

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestToolExecute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output":      base64.StdEncoding.EncodeToString([]byte("ok")),
			"exit_code":   0,
			"duration_ms": 7,
			"artifacts":   []string{},
		})
	}))
	defer server.Close()

	client := newToolClient(server.URL, time.Second)
	res, err := client.Execute(context.Background(), "echo", []byte("x"))
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if res.ExitCode != 0 || string(res.Output) != "ok" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestToolExecute_RequestFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/execute" {
			t.Errorf("expected /execute, got %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type: got %q", r.Header.Get("Content-Type"))
		}
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["name"] != "my_tool" {
			t.Errorf("name: got %v", req["name"])
		}
		// input is base64-encoded
		decoded, _ := base64.StdEncoding.DecodeString(req["input"].(string))
		if string(decoded) != "hello" {
			t.Errorf("input: got %q", decoded)
		}
		if req["timeout_seconds"].(float64) != 30 {
			t.Errorf("timeout_seconds: got %v", req["timeout_seconds"])
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output":      base64.StdEncoding.EncodeToString([]byte("result")),
			"exit_code":   0,
			"duration_ms": 5,
			"status":      "completed",
		})
	}))
	defer server.Close()

	client := newToolClient(server.URL, time.Second)
	res, err := client.Execute(context.Background(), "my_tool", []byte("hello"))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if string(res.Output) != "result" {
		t.Errorf("output: got %q", res.Output)
	}
	if res.Status != "completed" {
		t.Errorf("status: got %q", res.Status)
	}
}

func TestToolExecute_NonZeroExitCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output":      base64.StdEncoding.EncodeToString([]byte("error output")),
			"exit_code":   1,
			"duration_ms": 10,
			"status":      "failed",
		})
	}))
	defer server.Close()

	client := newToolClient(server.URL, time.Second)
	res, err := client.Execute(context.Background(), "tool", []byte("input"))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.ExitCode != 1 {
		t.Errorf("ExitCode: got %d", res.ExitCode)
	}
	if string(res.Output) != "error output" {
		t.Errorf("Output: got %q", res.Output)
	}
}

func TestToolExecute_WithArtifacts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output":      base64.StdEncoding.EncodeToString([]byte("")),
			"exit_code":   0,
			"duration_ms": 3,
			"artifacts":   []string{"gs://bucket/file1.json", "gs://bucket/file2.json"},
			"status":      "completed",
		})
	}))
	defer server.Close()

	client := newToolClient(server.URL, time.Second)
	res, err := client.Execute(context.Background(), "tool", nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(res.Artifacts) != 2 {
		t.Errorf("Artifacts: got %d items", len(res.Artifacts))
	}
}

func TestToolExecute_ApprovalRequestID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output":              "",
			"exit_code":           0,
			"duration_ms":         0,
			"status":              "pending_approval",
			"approval_request_id": "apr-123",
		})
	}))
	defer server.Close()

	client := newToolClient(server.URL, time.Second)
	res, err := client.Execute(context.Background(), "dangerous_tool", []byte("{}"))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.ApprovalRequestID != "apr-123" {
		t.Errorf("ApprovalRequestID: got %q", res.ApprovalRequestID)
	}
	if res.Status != "pending_approval" {
		t.Errorf("Status: got %q", res.Status)
	}
}

func TestToolExecutionResult_Struct(t *testing.T) {
	r := ToolExecutionResult{
		Output:            []byte("data"),
		ExitCode:          0,
		DurationMs:        42,
		Artifacts:         []string{"a", "b"},
		Status:            "completed",
		ApprovalRequestID: "apr-1",
	}
	if string(r.Output) != "data" {
		t.Errorf("Output: %q", r.Output)
	}
	if r.DurationMs != 42 {
		t.Errorf("DurationMs: %d", r.DurationMs)
	}
	if len(r.Artifacts) != 2 {
		t.Errorf("Artifacts: %v", r.Artifacts)
	}
}
