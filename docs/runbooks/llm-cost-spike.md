# Runbook: LLM Cost Spike

## Detect
- Alert: `AstraLLMCostSpike`
- Dashboard: LLM cost and token usage panel

## Triage
- Identify model and tenant/agent causing spike
- Confirm whether cache hit rate regressed

## Contain
- Force lower-cost model tier routing where possible
- Increase caching TTL for repeated prompts if safe

## Remediate
- Patch router model-selection policy
- Add safeguards on premium model use
- Review and adjust budget alerts
