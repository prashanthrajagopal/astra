package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type GoalClient struct {
	baseURL string
	client  *http.Client
}

type GoalCreateResponse struct {
	GoalID     string `json:"goal_id"`
	PhaseRunID string `json:"phase_run_id"`
	TaskCount  int    `json:"task_count"`
	GraphID    string `json:"graph_id"`
}

func NewGoalClient(baseURL string, timeout time.Duration) *GoalClient {
	return &GoalClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		client:  &http.Client{Timeout: timeout},
	}
}

func (g *GoalClient) CreateGoal(ctx context.Context, agentID, goalText string, priority int) (*GoalCreateResponse, error) {
	body, _ := json.Marshal(map[string]any{
		"agent_id":  agentID,
		"goal_text": goalText,
		"priority":  priority,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/goals", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("sdk.Goal.CreateGoal: unexpected status %d", resp.StatusCode)
	}
	var out GoalCreateResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (g *GoalClient) WaitForCompletion(ctx context.Context, goalID string, pollInterval time.Duration) (map[string]any, error) {
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}
	t := time.NewTicker(pollInterval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-t.C:
			resp, err := g.finalize(ctx, goalID)
			if err != nil {
				return nil, err
			}
			status, _ := resp["status"].(string)
			if status == "completed" || status == "failed" {
				return resp, nil
			}
		}
	}
}

func (g *GoalClient) finalize(ctx context.Context, goalID string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/goals/"+goalID+"/finalize", nil)
	if err != nil {
		return nil, err
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("sdk.Goal.finalize: unexpected status %d", resp.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}
