package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"astra/pkg/sdk"
)

type SimpleAgent struct{}

func (a *SimpleAgent) Plan(ctx context.Context, goal string) []sdk.TaskSpec {
	return []sdk.TaskSpec{{
		Type:     "analyze",
		Payload:  []byte(fmt.Sprintf(`{"goal":%q}`, goal)),
		Priority: 100,
	}}
}

func (a *SimpleAgent) Execute(ctx context.Context, agentCtx sdk.AgentContext, t sdk.TaskSpec) error {
	res, err := agentCtx.CallTool(ctx, "echo simple-agent", t.Payload)
	if err != nil {
		return err
	}
	if res.Status == "pending_approval" {
		log.Printf("tool call pending approval: %s", res.ApprovalRequestID)
	}
	return nil
}

func (a *SimpleAgent) Reflect(ctx context.Context, agentCtx sdk.AgentContext, summary string) error {
	_, err := agentCtx.Memory().Write(ctx, agentCtx.ID(), "reflection", summary, nil)
	return err
}

func main() {
	agentCtx, err := sdk.NewAgentContext(sdk.DefaultConfig())
	if err != nil {
		log.Fatalf("new agent context: %v", err)
	}
	defer agentCtx.Close()

	a := &SimpleAgent{}
	goal := "demonstrate plan execute reflect loop"
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	tasks := a.Plan(ctx, goal)
	for _, t := range tasks {
		taskID, err := agentCtx.CreateTask(ctx, t)
		if err != nil {
			log.Fatalf("create task: %v", err)
		}
		if err := a.Execute(ctx, agentCtx, t); err != nil {
			log.Fatalf("execute task %s: %v", taskID, err)
		}
	}

	reflection, _ := json.Marshal(map[string]any{"goal": goal, "status": "done"})
	if err := a.Reflect(ctx, agentCtx, string(reflection)); err != nil {
		log.Printf("reflect write failed (ensure memory-service and spawned agent persistence): %v", err)
	}

	log.Printf("simple-agent completed for agent_id=%s", agentCtx.ID())
}
