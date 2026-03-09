# Runbook: Worker Lost

## Detect
- Alert: `AstraWorkerAvailabilityLow`
- Symptom: no worker heartbeats, queue growth

## Triage
- Check worker-manager health and `/workers` output
- Check `logs/execution-worker.log` and `logs/browser-worker.log`
- Verify Redis stream activity for `astra:tasks:shard:0`

## Contain
- Mark stale workers offline (worker-manager loop)
- Re-queue orphaned tasks

## Remediate
- Restart worker processes via `scripts/deploy.sh`
- Verify heartbeats recover and queue depth decreases
