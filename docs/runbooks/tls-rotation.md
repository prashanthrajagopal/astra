# TLS certificate rotation

## Trigger

- Certificate expiration alert
- Certificate key compromise
- Planned certificate authority rollover

## Steps

1. Generate replacement server and client certs signed by approved CA.
2. Update Kubernetes secret or host-mounted cert files referenced by:
   - `ASTRA_TLS_CERT_FILE`
   - `ASTRA_TLS_KEY_FILE`
   - `ASTRA_TLS_CA_FILE`
3. Roll services in dependency order:
   - gRPC servers (`agent-service`, `task-service`, `memory-service`, `llm-router`)
   - HTTP services (`identity`, `access-control`, `tool-runtime`, `goal-service`, `planner-service`, `evaluation-service`, `prompt-manager`, `worker-manager`, `api-gateway`)
4. Verify health endpoints and mTLS connectivity.
5. Revoke old certificates and record rotation in incident log.

## Verification

- `scripts/validate.sh` Phase 7 checks pass
- service logs show successful TLS handshakes
- no certificate expiration alerts remain
