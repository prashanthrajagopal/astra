package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"astra/internal/agentdocs"
	"astra/internal/planner"
	"astra/internal/tasks"

	"github.com/google/uuid"
)

const (
	retryBatchSize  = 50
	retryConcurrent = 10
	retryInterval   = 5 * time.Second
)

// retryPendingGoals runs a background loop that picks up pending goals.
func (s *goalServer) retryPendingGoals(ctx context.Context) {
	ticker := time.NewTicker(retryInterval)
	defer ticker.Stop()
	// Apply any already-approved plans before doing new planning.
	s.applyApprovedApprovals(ctx)
	s.processPendingGoals(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.applyApprovedApprovals(ctx)
			s.processPendingGoals(ctx)
		}
	}
}

// applyApprovedApprovals processes all approval_requests with status='approved' in batches,
// creating task graphs and activating the associated goals.
func (s *goalServer) applyApprovedApprovals(ctx context.Context) {
	for {
		rows, err := s.db.QueryContext(ctx, `
			SELECT id, plan_payload FROM approval_requests
			WHERE status = 'approved' AND request_type = 'plan'
			ORDER BY created_at ASC
			LIMIT 100
		`)
		if err != nil {
			slog.Warn("apply-approvals: query failed", "err", err)
			return
		}

		type row struct {
			id          uuid.UUID
			planPayload []byte
		}
		var batch []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.id, &r.planPayload); err != nil {
				continue
			}
			batch = append(batch, r)
		}
		rows.Close()

		if len(batch) == 0 {
			return
		}

		applied := 0
		for _, r := range batch {
			if err := s.applyApproval(ctx, r.id, r.planPayload); err != nil {
				slog.Warn("apply-approvals: apply failed", "approval_id", r.id, "err", err)
				// Mark as failed so it doesn't block the batch loop.
				_, _ = s.db.ExecContext(ctx,
					`UPDATE approval_requests SET status='failed', updated_at=now() WHERE id=$1`, r.id)
			} else {
				applied++
			}
		}
		slog.Info("apply-approvals: batch done", "applied", applied, "total", len(batch))
	}
}

func (s *goalServer) applyApproval(ctx context.Context, approvalID uuid.UUID, planPayload []byte) error {
	var spec planPayloadSpec
	if err := json.Unmarshal(planPayload, &spec); err != nil {
		return err
	}
	graphID, err := uuid.Parse(spec.GraphID)
	if err != nil {
		return err
	}
	goalID, err := uuid.Parse(spec.GoalID)
	if err != nil {
		return err
	}
	agentID, err := uuid.Parse(spec.AgentID)
	if err != nil {
		return err
	}
	taskList := make([]tasks.Task, len(spec.Tasks))
	for i := range spec.Tasks {
		pt := &spec.Tasks[i]
		taskID, err := uuid.Parse(pt.ID)
		if err != nil {
			return err
		}
		payload := pt.Payload
		if payload == nil {
			payload = []byte("{}")
		}
		taskList[i] = tasks.Task{
			ID:         taskID,
			GraphID:    graphID,
			GoalID:     goalID,
			AgentID:    agentID,
			Type:       pt.Type,
			Status:     tasks.StatusCreated,
			Payload:    payload,
			Priority:   pt.Priority,
			MaxRetries: pt.MaxRetries,
		}
	}
	var deps []tasks.Dependency
	for _, d := range spec.Dependencies {
		taskID, err1 := uuid.Parse(d.TaskID)
		dependsOn, err2 := uuid.Parse(d.DependsOn)
		if err1 != nil || err2 != nil {
			continue
		}
		deps = append(deps, tasks.Dependency{TaskID: taskID, DependsOn: dependsOn})
	}
	graph := tasks.Graph{ID: graphID, Tasks: taskList, Dependencies: deps}
	if err := s.taskStore.CreateGraph(ctx, &graph); err != nil {
		return err
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE goals SET status='active' WHERE id=$1`, goalID)
	_, _ = s.db.ExecContext(ctx,
		`UPDATE approval_requests SET status='applied', decided_at=now(), updated_at=now() WHERE id=$1`, approvalID)
	return nil
}

func (s *goalServer) processPendingGoals(ctx context.Context) {
	var activePlanning int
	_ = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM phase_runs
		WHERE status = 'running' AND started_at > now() - interval '5 minutes'
	`).Scan(&activePlanning)
	if activePlanning >= retryConcurrent {
		return
	}
	limit := retryConcurrent - activePlanning

	rows, err := s.db.QueryContext(ctx, `
		SELECT g.id, g.agent_id, g.goal_text, COALESCE(g.workspace_tag, '')
		FROM goals g
		WHERE g.status = 'pending'
		AND NOT EXISTS (
			SELECT 1 FROM phase_runs pr
			WHERE pr.goal_id = g.id
			AND pr.status = 'running'
			AND pr.started_at > now() - interval '5 minutes'
		)
		AND NOT EXISTS (
			SELECT 1 FROM approval_requests ar
			WHERE ar.goal_id = g.id
			AND ar.status = 'pending'
		)
		ORDER BY g.priority DESC, g.created_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		slog.Warn("retry: query pending goals failed", "err", err)
		return
	}
	defer rows.Close()

	type pendingGoal struct {
		id           uuid.UUID
		agentID      uuid.UUID
		goalText     string
		workspaceTag string
	}
	var pending []pendingGoal
	for rows.Next() {
		var g pendingGoal
		if err := rows.Scan(&g.id, &g.agentID, &g.goalText, &g.workspaceTag); err != nil {
			slog.Warn("retry: scan goal failed", "err", err)
			continue
		}
		pending = append(pending, g)
	}
	rows.Close()

	sem := make(chan struct{}, retryConcurrent)
	var wg sync.WaitGroup
	for _, g := range pending {
		g := g
		// Close any stale running phase_runs before re-attempting.
		_, _ = s.db.ExecContext(ctx,
			`UPDATE phase_runs SET status='failed', ended_at=now(), updated_at=now()
			 WHERE goal_id=$1 AND status='running'`, g.id)

		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if err := s.planGoal(ctx, g.id, g.agentID, g.goalText, g.workspaceTag); err != nil {
				slog.Warn("retry: plan failed", "goal_id", g.id, "err", err)
			} else {
				slog.Info("retry: goal planned", "goal_id", g.id)
			}
		}()
	}
	wg.Wait()
}

// planGoal runs the planner for an existing pending goal and either enqueues tasks
// (when autoApprovePlans is true) or creates a pending approval request.
func (s *goalServer) planGoal(ctx context.Context, goalID, agentID uuid.UUID, goalText, workspaceTag string) error {
	phaseRunID := uuid.New()
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO phase_runs (id, goal_id, agent_id, status) VALUES ($1, $2, $3, 'running')`,
		phaseRunID, goalID, agentID); err != nil {
		return err
	}

	var actorType string
	_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(actor_type, '') FROM agents WHERE id = $1`, agentID).Scan(&actorType)

	agentCtx, err := s.docStore.AssembleContext(ctx, agentID, &goalID)
	var agentCtxJSON json.RawMessage
	if err == nil && agentCtx != nil {
		agentCtxJSON, _ = agentdocs.SerializeContext(agentCtx)
	}

	planOpts := &planner.PlanOptions{Workspace: workspaceTag, AgentContext: agentCtxJSON, ActorType: actorType}
	if actorType != "" && os.Getenv(strings.ToUpper(actorType)+"_ADAPTER_ADDR") != "" {
		planOpts.DefaultTaskType = "adapter_dispatch"
	}
	graph, err := s.planner.Plan(ctx, goalID, goalText, agentID, planOpts)
	if err != nil {
		_, _ = s.db.ExecContext(ctx,
			`UPDATE phase_runs SET status='failed', ended_at=now(), updated_at=now() WHERE id=$1`, phaseRunID)
		return err
	}

	if !s.autoApprovePlans {
		planPayload := buildPlanPayload(&graph, goalID, agentID, goalText)
		planPayloadJSON, _ := json.Marshal(planPayload)
		approvalID := uuid.New()
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO approval_requests (id, request_type, goal_id, graph_id, plan_payload, status, requested_by)
			 VALUES ($1, 'plan', $2, $3, $4, 'pending', $5)`,
			approvalID, goalID, graph.ID, planPayloadJSON, "system"); err != nil {
			return err
		}
		assignApprovalToAdmin(ctx, s.db, approvalID, agentID)
		// Close the phase_run so the retry loop does not re-pick this goal up.
		_, _ = s.db.ExecContext(ctx,
			`UPDATE phase_runs SET status='completed', ended_at=now(), updated_at=now() WHERE id=$1`, phaseRunID)
		return nil
	}

	if err := s.taskStore.CreateGraph(ctx, &graph); err != nil {
		return err
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE goals SET status='active' WHERE id=$1`, goalID)
	return nil
}
