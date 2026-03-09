# Astra Helm Chart

Phase 5 hardening includes:

- Resource requests/limits via `values.yaml`
- HorizontalPodAutoscaler template (`templates/hpa.yaml`)
- PodDisruptionBudget template (`templates/pdb.yaml`)

## Validate

```bash
helm template astra deployments/helm/astra
```
