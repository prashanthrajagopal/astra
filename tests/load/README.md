# Phase 5 Load Test Plan

This directory contains load-test assets for Astra Phase 5 hardening.

## Scenarios

- **Smoke:** low concurrency sanity check
- **Sustained:** moderate constant request load
- **Burst:** short high-load spikes

Targets from PRD:

- Up to 10k agents and 1M tasks/day equivalent workload
- p99 read latency <= 10ms (cache-hot paths)
- Scheduling median <= 50ms, p95 <= 500ms

## Run

Install [k6](https://k6.io/), then:

```bash
k6 run tests/load/k6-config.js
```

Optionally override env vars:

- `BASE_URL` (default `http://localhost:8080`)
- `JWT_TOKEN` (Bearer token for protected endpoints)

## Results

Store run outputs and conclusions in `tests/load/results/`.
If SLOs are not met, add remediation notes in this README.
