# Runbook: High Error Rate

## Detect
- Alert: `AstraHighTaskFailureRate`
- Symptom: rising failed task count or API 5xx

## Triage
- Inspect `logs/*` for failing component
- Check recent deploy/commit and changed configs
- Review task payloads causing failures

## Contain
- Reduce intake rate temporarily
- Route critical paths to noop/backoff mode if needed

## Remediate
- Roll forward with hotfix or rollback to last known stable build
- Re-run `scripts/validate.sh`
