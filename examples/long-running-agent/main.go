package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"time"

	"astra/pkg/sdk"
)

type PRDStage struct {
	Name      string
	GoalText  string
	ToolName  string
	ToolInput map[string]any
}

func main() {
	cycleCount := flag.Int("cycles", 2, "number of full PRD cycles to run")
	pauseBetweenStages := flag.Duration("stage-pause", 4*time.Second, "pause between stages")
	pollInterval := flag.Duration("poll-interval", 3*time.Second, "goal finalize poll interval")
	goalServiceAddr := flag.String("goal-service", "http://localhost:8088", "goal service base URL")
	flag.Parse()

	cfg := sdk.DefaultConfig()
	agentCtx, err := sdk.NewAgentContext(cfg)
	if err != nil {
		log.Fatalf("new agent context: %v", err)
	}
	defer agentCtx.Close()

	goalClient := sdk.NewGoalClient(*goalServiceAddr, 5*time.Second)
	stages := prdStages()
	log.Printf("long-running-agent started: agent_id=%s cycles=%d stages=%d", agentCtx.ID(), *cycleCount, len(stages))

	for cycle := 1; cycle <= *cycleCount; cycle++ {
		log.Printf("cycle %d/%d started", cycle, *cycleCount)
		for i, stage := range stages {
			if err := runStage(agentCtx, goalClient, cycle, i+1, len(stages), stage, *pollInterval); err != nil {
				log.Printf("stage failed (continuing): cycle=%d stage=%s err=%v", cycle, stage.Name, err)
			}
			if i < len(stages)-1 {
				time.Sleep(*pauseBetweenStages)
			}
		}
		log.Printf("cycle %d/%d completed", cycle, *cycleCount)
	}

	summary := map[string]any{
		"status":      "completed",
		"agent_id":    agentCtx.ID(),
		"cycles":      *cycleCount,
		"stages":      len(stages),
		"finished_at": time.Now().UTC().Format(time.RFC3339),
	}
	if err := writeMemory(context.Background(), agentCtx, "run-summary", summary); err != nil {
		log.Printf("summary memory write failed: %v", err)
	}
	log.Printf("long-running-agent finished")
}

func runStage(agentCtx sdk.AgentContext, goalClient *sdk.GoalClient, cycle, idx, total int, stage PRDStage, pollInterval time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	startPayload := map[string]any{
		"cycle":      cycle,
		"stage":      idx,
		"total":      total,
		"stage_name": stage.Name,
		"goal":       stage.GoalText,
		"started_at": time.Now().UTC().Format(time.RFC3339),
	}
	if _, err := agentCtx.PublishEvent(ctx, sdk.Event{
		StreamName: "astra:events",
		EventType:  "LongRunStageStarted",
		ActorID:    agentCtx.ID(),
		Payload:    mustJSON(startPayload),
	}); err != nil {
		log.Printf("publish start event failed: %v", err)
	}
	if err := writeMemory(ctx, agentCtx, "stage-plan", startPayload); err != nil {
		log.Printf("write stage-plan memory failed: %v", err)
	}

	goalResp, err := goalClient.CreateGoal(ctx, agentCtx.ID(), stage.GoalText, 100)
	if err != nil {
		return fmt.Errorf("create goal: %w", err)
	}
	log.Printf("goal created: cycle=%d stage=%s goal_id=%s phase_run_id=%s task_count=%d", cycle, stage.Name, goalResp.GoalID, goalResp.PhaseRunID, goalResp.TaskCount)

	if stage.ToolName != "" {
		toolRes, toolErr := agentCtx.CallTool(ctx, stage.ToolName, mustJSON(stage.ToolInput))
		if toolErr != nil {
			log.Printf("tool call failed: stage=%s err=%v", stage.Name, toolErr)
		} else if toolRes.Status == "pending_approval" {
			log.Printf("tool pending approval: stage=%s approval_request_id=%s", stage.Name, toolRes.ApprovalRequestID)
		} else {
			log.Printf("tool executed: stage=%s exit_code=%d", stage.Name, toolRes.ExitCode)
		}
	}

	finalResp, err := goalClient.WaitForCompletion(ctx, goalResp.GoalID, pollInterval)
	if err != nil {
		return fmt.Errorf("wait for completion: %w", err)
	}

	finalPayload := map[string]any{
		"cycle":        cycle,
		"stage_name":   stage.Name,
		"goal_id":      goalResp.GoalID,
		"phase_run_id": goalResp.PhaseRunID,
		"final":        finalResp,
		"finished_at":  time.Now().UTC().Format(time.RFC3339),
	}
	if err := writeMemory(ctx, agentCtx, "stage-result", finalPayload); err != nil {
		log.Printf("write stage-result memory failed: %v", err)
	}
	if _, err := agentCtx.PublishEvent(ctx, sdk.Event{
		StreamName: "astra:events",
		EventType:  "LongRunStageFinished",
		ActorID:    agentCtx.ID(),
		Payload:    mustJSON(finalPayload),
	}); err != nil {
		log.Printf("publish finish event failed: %v", err)
	}
	return nil
}

func writeMemory(ctx context.Context, agentCtx sdk.AgentContext, memoryType string, payload map[string]any) error {
	_, err := agentCtx.Memory().Write(ctx, agentCtx.ID(), memoryType, string(mustJSON(payload)), nil)
	return err
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

func prdStages() []PRDStage {
	return []PRDStage{
		{
			Name:     "architecture",
			GoalText: "Translate PRD architecture section into a task DAG and execution plan",
			ToolName: "echo architecture-plan",
			ToolInput: map[string]any{
				"section": "architecture",
				"output":  "dag",
			},
		},
		{
			Name:     "security",
			GoalText: "Validate security requirements and run gated tool checks for risky operations",
			ToolName: "terraform plan", // intentionally risky to trigger approvals in some environments
			ToolInput: map[string]any{
				"section": "security",
				"policy":  "strict",
			},
		},
		{
			Name:     "observability",
			GoalText: "Generate observability verification tasks and summarize rollout readiness",
			ToolName: "echo observability-check",
			ToolInput: map[string]any{
				"section": "observability",
				"target":  "production",
			},
		},
	}
}
