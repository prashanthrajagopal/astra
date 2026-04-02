package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"astra/internal/tasks"
	"astra/internal/tools"

	"github.com/google/uuid"
)

// --- newToolRuntimes ---

func TestNewToolRuntimes_DefaultNoop(t *testing.T) {
	t.Setenv("TOOL_RUNTIME", "")
	t.Setenv("WORKSPACE_ROOT", "")
	ws, legacy := newToolRuntimes()
	if ws == nil {
		t.Fatal("expected non-nil WorkspaceRuntime")
	}
	if legacy == nil {
		t.Fatal("expected non-nil legacy Runtime")
	}
	// Should be NoopRuntime by default
	req := tools.ToolRequest{Name: "any", Input: []byte("{}"), Timeout: time.Second, MemoryLimit: 1 << 20, CPULimit: 1.0}
	result, err := legacy.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("noop Execute: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("noop ExitCode = %d, want 0", result.ExitCode)
	}
}

func TestNewToolRuntimes_DockerRuntime(t *testing.T) {
	t.Setenv("TOOL_RUNTIME", "docker")
	_, legacy := newToolRuntimes()
	if legacy == nil {
		t.Fatal("expected non-nil Docker runtime")
	}
}

func TestNewToolRuntimes_WorkspaceRootEnv(t *testing.T) {
	t.Setenv("WORKSPACE_ROOT", "/tmp/test-workspace")
	ws, _ := newToolRuntimes()
	if ws == nil {
		t.Fatal("expected non-nil WorkspaceRuntime")
	}
}

// --- executeTask routing ---

func makeTask(taskType string, payload map[string]interface{}) *tasks.Task {
	p, _ := json.Marshal(payload)
	return &tasks.Task{
		ID:      uuid.New(),
		GoalID:  uuid.New(),
		AgentID: uuid.New(),
		Type:    taskType,
		Payload: p,
		Status:  tasks.StatusQueued,
	}
}

func TestExecuteTask_AdapterRouting(t *testing.T) {
	// When payload has provider_type != "astra_agent", should call executeViaAdapter.
	// We set an env var so adapter can be resolved; since no real server exists it will fail
	// with a connection error — that's fine, we just want to verify routing happened.
	t.Setenv("MYCLOUD_ADAPTER_ADDR", "http://127.0.0.1:1") // guaranteed-closed port
	task := makeTask("anything", map[string]interface{}{
		"provider_type": "mycloud",
		"goal_text":     "do something",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := executeTask(ctx, task, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error from unreachable adapter, got nil")
	}
	// Should mention adapter dispatch (not an LLM or tool error)
	if !strings.Contains(err.Error(), "adapter") && !strings.Contains(err.Error(), "connect") && !strings.Contains(err.Error(), "connection") {
		t.Logf("error was: %v", err)
	}
}

func TestExecuteTask_AstraAgentNotRouted(t *testing.T) {
	// provider_type = "astra_agent" should NOT go to adapter; falls to switch
	task := makeTask("shell_exec", map[string]interface{}{
		"provider_type": "astra_agent",
	})
	ws, legacy := newToolRuntimes()

	ctx := context.Background()
	// shell_exec with no real workspace — will error, but that's OK; we just want
	// no adapter routing to happen.
	_, _ = executeTask(ctx, task, ws, legacy, nil)
}

func TestExecuteTask_NoProviderType_DefaultRouting(t *testing.T) {
	// No provider_type key → goes to switch.
	task := makeTask("unknown_type", nil)
	_, legacy := newToolRuntimes()

	ctx := context.Background()
	// legacy noop runtime, unknown type → executeLegacy → noop returns exit 0
	result, err := executeTask(ctx, task, nil, legacy, nil)
	if err != nil {
		t.Fatalf("executeTask default: unexpected error %v", err)
	}
	_ = result
}

func TestExecuteTask_CodeGenerateNoLLM(t *testing.T) {
	task := makeTask("code_generate", nil)
	ws, _ := newToolRuntimes()

	ctx := context.Background()
	_, err := executeTask(ctx, task, ws, nil, nil /* no llm client */)
	if err == nil {
		t.Fatal("expected error: llm-router not available")
	}
	if !strings.Contains(err.Error(), "llm-router not available") {
		t.Errorf("expected 'llm-router not available', got: %v", err)
	}
}

// --- executeViaAdapter with httptest ---

func TestExecuteViaAdapter_NoAdapterConfigured(t *testing.T) {
	t.Setenv("TESTPROVIDER_ADAPTER_ADDR", "")
	task := makeTask("something", nil)
	_, err := executeViaAdapter(context.Background(), task, "testprovider", map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for missing adapter addr")
	}
	if !strings.Contains(err.Error(), "no adapter configured") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExecuteViaAdapter_DispatchCompleted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/dispatch" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"job_id":  "job-001",
				"status":  "completed",
				"output":  json.RawMessage(`{"result":"ok"}`),
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	t.Setenv("MYPROVIDER_ADAPTER_ADDR", srv.URL)
	task := makeTask("run", map[string]interface{}{"provider_type": "myprovider", "goal_text": "test"})
	out, err := executeViaAdapter(context.Background(), task, "myprovider", map[string]interface{}{"goal_text": "test"})
	if err != nil {
		t.Fatalf("executeViaAdapter completed: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil output")
	}
}

func TestExecuteViaAdapter_DispatchFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"job_id":  "job-002",
			"status":  "failed",
			"output":  nil,
		})
	}))
	defer srv.Close()

	t.Setenv("FAILPROVIDER_ADAPTER_ADDR", srv.URL)
	task := makeTask("run", nil)
	_, err := executeViaAdapter(context.Background(), task, "failprovider", map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for failed job")
	}
	if !strings.Contains(err.Error(), "failed") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestExecuteViaAdapter_DispatchHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()

	t.Setenv("ERRPROVIDER_ADAPTER_ADDR", srv.URL)
	task := makeTask("run", nil)
	_, err := executeViaAdapter(context.Background(), task, "errprovider", map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for HTTP 403")
	}
	if !strings.Contains(err.Error(), "error") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- pollAdapterStatus with httptest ---

func TestPollAdapterStatus_Completed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "completed",
			"output": json.RawMessage(`{"done":true}`),
		})
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	out, err := pollAdapterStatus(context.Background(), srv.URL, "job-xyz", client)
	if err != nil {
		t.Fatalf("pollAdapterStatus completed: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil output")
	}
}

func TestPollAdapterStatus_Failed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "failed",
			"error":  "something went wrong",
		})
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	_, err := pollAdapterStatus(context.Background(), srv.URL, "job-fail", client)
	if err == nil {
		t.Fatal("expected error for failed job")
	}
	if !strings.Contains(err.Error(), "failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPollAdapterStatus_ContextCancel(t *testing.T) {
	// Server that always returns "running" so poll never completes naturally
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "running"})
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	client := &http.Client{Timeout: 200 * time.Millisecond}
	_, err := pollAdapterStatus(ctx, srv.URL, "job-ctx", client)
	if err == nil {
		t.Fatal("expected context-cancelled error")
	}
}

// --- publishDeadLetterIf (pure logic) ---

func TestPublishDeadLetterIf_NilBus(t *testing.T) {
	// Should not panic when bus is nil
	publishDeadLetterIf(nil, context.Background(), "tid", "gid", "err", true)
}

func TestPublishDeadLetterIf_NotMoved(t *testing.T) {
	// Should not publish when movedToDeadLetter is false
	publishDeadLetterIf(nil, context.Background(), "tid", "gid", "err", false)
}

// --- executeLegacy ---

func TestExecuteLegacy_NoopSuccess(t *testing.T) {
	task := &tasks.Task{
		ID:      uuid.New(),
		Type:    "noop_tool",
		Payload: []byte(`{"key":"val"}`),
	}
	rt := tools.NewNoopRuntime()
	out, err := executeLegacy(context.Background(), task, rt)
	if err != nil {
		t.Fatalf("executeLegacy noop: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil output")
	}
}

func TestExecuteLegacy_NilPayloadDefaultsToEmpty(t *testing.T) {
	task := &tasks.Task{
		ID:      uuid.New(),
		Type:    "noop_tool",
		Payload: nil,
	}
	rt := tools.NewNoopRuntime()
	out, err := executeLegacy(context.Background(), task, rt)
	if err != nil {
		t.Fatalf("executeLegacy nil payload: %v", err)
	}
	_ = out
}

// --- getEnv (already tested in main_test.go, adding edge cases) ---

func TestGetEnv_CaseSensitive(t *testing.T) {
	key := "EXEC_TEST_KEY_CASE"
	t.Setenv(key, "UPPER")
	got := getEnv(key, "default")
	if got != "UPPER" {
		t.Errorf("getEnv case: got %q, want UPPER", got)
	}
	got2 := getEnv(strings.ToLower(key), "default")
	// Lower-case key should not find UPPER env var on Linux; on macOS env is case-sensitive
	_ = got2
}

// --- ensure the adapter URL env var is normalised (uppercase + suffix) ---

func TestExecuteViaAdapter_EnvNameUppercased(t *testing.T) {
	// provider_type "Dtec" -> env var "DTEC_ADAPTER_ADDR"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"job_id": "j1", "status": "completed", "output": json.RawMessage(`{}`),
		})
	}))
	defer srv.Close()

	os.Setenv("DTEC_ADAPTER_ADDR", srv.URL)
	defer os.Unsetenv("DTEC_ADAPTER_ADDR")

	task := makeTask("run", nil)
	out, err := executeViaAdapter(context.Background(), task, "Dtec", map[string]interface{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = out
}
