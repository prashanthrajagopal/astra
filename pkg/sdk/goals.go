package sdk

import (
	"context"
	"encoding/json"
	"fmt"
)

// PostGoalRequest is the request to create a goal for another agent.
type PostGoalRequest struct {
	TargetAgentID    string   `json:"agent_id"`
	GoalText         string   `json:"goal_text"`
	Priority         int      `json:"priority,omitempty"`
	CascadeID        string   `json:"cascade_id,omitempty"`
	DependsOnGoalIDs []string `json:"depends_on_goal_ids,omitempty"`
}

// PostGoalResponse is the response from creating a goal.
type PostGoalResponse struct {
	GoalID            string `json:"goal_id"`
	ApprovalRequestID string `json:"approval_request_id,omitempty"`
	Status            string `json:"status"`
}

// GoalStatus represents the current state of a goal.
type GoalStatus struct {
	ID               string   `json:"id"`
	AgentID          string   `json:"agent_id"`
	GoalText         string   `json:"goal_text"`
	Priority         int      `json:"priority"`
	Status           string   `json:"status"`
	CascadeID        string   `json:"cascade_id,omitempty"`
	DependsOnGoalIDs []string `json:"depends_on_goal_ids,omitempty"`
	SourceAgentID    string   `json:"source_agent_id,omitempty"`
	CreatedAt        string   `json:"created_at"`
	CompletedAt      string   `json:"completed_at,omitempty"`
}

// PostGoal creates a goal for another agent (agent-to-agent goal posting).
func (c *HTTPClient) PostGoal(ctx context.Context, req PostGoalRequest) (*PostGoalResponse, error) {
	priority := req.Priority
	if priority <= 0 {
		priority = 100
	}

	body := map[string]interface{}{
		"agent_id":        req.TargetAgentID,
		"goal_text":       req.GoalText,
		"priority":        priority,
		"source_agent_id": c.agentID,
	}
	if req.CascadeID != "" {
		body["cascade_id"] = req.CascadeID
	}
	if len(req.DependsOnGoalIDs) > 0 {
		body["depends_on_goal_ids"] = req.DependsOnGoalIDs
	}

	resp, err := c.doRequest(ctx, "POST", "/internal/goals", body)
	if err != nil {
		return nil, fmt.Errorf("sdk.PostGoal: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var errResp struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		return nil, fmt.Errorf("sdk.PostGoal: %s (status %d)", errResp.Error, resp.StatusCode)
	}

	var result PostGoalResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("sdk.PostGoal: decode response: %w", err)
	}
	return &result, nil
}

// GetGoal retrieves the current state of a goal.
func (c *HTTPClient) GetGoal(ctx context.Context, goalID string) (*GoalStatus, error) {
	resp, err := c.doRequest(ctx, "GET", "/goals/"+goalID, nil)
	if err != nil {
		return nil, fmt.Errorf("sdk.GetGoal: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("sdk.GetGoal: status %d", resp.StatusCode)
	}

	var result GoalStatus
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("sdk.GetGoal: decode: %w", err)
	}
	return &result, nil
}
