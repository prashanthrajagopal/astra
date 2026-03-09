# Long-Running Agent Example

This example simulates a multi-stage PRD execution loop over multiple cycles.

It demonstrates:

- goal creation through `goal-service`
- polling/finalization over long-running cycles
- memory journaling (`stage-plan`, `stage-result`, `run-summary`)
- event publishing (`LongRunStageStarted`, `LongRunStageFinished`)
- tool calls including a risky tool (`terraform plan`) to trigger approvals

## Run

```bash
./scripts/deploy.sh

go run ./examples/long-running-agent
```

Optional flags:

```bash
go run ./examples/long-running-agent --cycles 3 --stage-pause 5s --poll-interval 2s
```

## Observe while it runs

- Dashboard: `http://localhost:8080/dashboard/`
- Pending approvals section for risky tool actions
- Logs in `logs/*.log`
