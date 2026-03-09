package sdk

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type ToolClient interface {
	Execute(ctx context.Context, name string, input []byte) (ToolExecutionResult, error)
}

type httpToolClient struct {
	baseURL string
	client  *http.Client
}

func newToolClient(baseURL string, timeout time.Duration) ToolClient {
	return &httpToolClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: timeout},
	}
}

func (t *httpToolClient) Execute(ctx context.Context, name string, input []byte) (ToolExecutionResult, error) {
	payload := map[string]any{
		"name":            name,
		"input":           base64.StdEncoding.EncodeToString(input),
		"timeout_seconds": 30,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/execute", bytes.NewReader(body))
	if err != nil {
		return ToolExecutionResult{}, fmt.Errorf("sdk.Tool.Execute request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return ToolExecutionResult{}, fmt.Errorf("sdk.Tool.Execute call: %w", err)
	}
	defer resp.Body.Close()

	var out struct {
		Output            string   `json:"output"`
		ExitCode          int      `json:"exit_code"`
		DurationMs        int64    `json:"duration_ms"`
		Artifacts         []string `json:"artifacts"`
		Status            string   `json:"status"`
		ApprovalRequestID string   `json:"approval_request_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return ToolExecutionResult{}, fmt.Errorf("sdk.Tool.Execute decode: %w", err)
	}

	decoded, _ := base64.StdEncoding.DecodeString(out.Output)
	return ToolExecutionResult{
		Output:            decoded,
		ExitCode:          out.ExitCode,
		DurationMs:        out.DurationMs,
		Artifacts:         out.Artifacts,
		Status:            out.Status,
		ApprovalRequestID: out.ApprovalRequestID,
	}, nil
}
