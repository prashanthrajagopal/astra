# Astra Helm Chart

Phase 5 hardening includes:

- Resource requests/limits via `values.yaml`
- HorizontalPodAutoscaler template (`templates/hpa.yaml`)
- PodDisruptionBudget template (`templates/pdb.yaml`)

Phase 7 security additions:

- TLS environment wiring (`ASTRA_TLS_*`) through chart values
- Optional TLS secret mount (`tls.secretName`) at `/etc/astra/tls`
- Vault environment wiring (`ASTRA_VAULT_*`) for runtime secret overlay

## Validate

```bash
helm template astra deployments/helm/astra
```
