# Astra Go SDK (`pkg/sdk`)

This package provides a stable application-facing SDK without importing `internal/*`.

## Core interfaces

- `AgentContext`
- `MemoryClient`
- `ToolClient`

## Quick start

```go
cfg := sdk.DefaultConfig()
ctx, err := sdk.NewAgentContext(cfg)
if err != nil { panic(err) }
defer ctx.Close()

taskID, _ := ctx.CreateTask(context.Background(), sdk.TaskSpec{
    Type: "analyze",
    Payload: []byte(`{"goal":"build report"}`),
    Priority: 100,
})
_ = taskID
```

## Service dependencies

By default SDK connects to:

- Kernel gRPC: `localhost:9091`
- Task gRPC: `localhost:9090`
- Memory gRPC: `localhost:9092`
- Tool runtime HTTP: `http://localhost:8083`

## Optional goal helpers

`GoalClient` (in `goal.go`) can call goal-service directly:

- `CreateGoal(agentID, goalText, priority)`
- `WaitForCompletion(goalID, pollInterval)`
