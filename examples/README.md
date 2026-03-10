# SDK Examples

- `examples/simple-agent` — plan/execute/reflect flow
- `examples/echo-agent` — minimal tool call with SDK
- `examples/long-running-agent` — multi-stage PRD execution loop with goal polling, memory journaling, and approvals
- `examples/ecommerce-test` — **real-world test**: autonomous agent builds a full Next.js e-commerce site

Run examples after deploying Astra services:

```bash
./scripts/deploy.sh
go run ./examples/simple-agent
go run ./examples/echo-agent
go run ./examples/long-running-agent
bash examples/ecommerce-test/run.sh   # full e-commerce build test
```
