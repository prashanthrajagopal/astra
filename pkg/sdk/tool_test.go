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
