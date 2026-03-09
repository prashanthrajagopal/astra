# Deploy

Run deployment for Astra. **Only the DevOps agent** may execute deployment.

## Routing

- **Delegate to:** DevOps Engineer (do not run deploy script or Helm yourself unless you are DevOps).
- **Local:** DevOps runs `scripts/deploy.sh` from the repo root.
- **Cloud:** DevOps follows `.cursor/skills/devops-deployment/SKILL.md` (Helm upgrade/install with `deployments/helm/astra`).

## Steps (for DevOps agent)

1. Read `.cursor/skills/devops-deployment/SKILL.md` for the full runbook.
2. **Local deployment:** From repo root, run `./scripts/deploy.sh`. Report outcome: which services used native vs Docker, log paths, how to stop.
3. **Cloud deployment:** Use `helm upgrade --install astra ./deployments/helm/astra` with the appropriate namespace and values file; document any overrides. Run rollback steps if the user requests rollback.

## For non-DevOps agents

If the user asks you to "deploy" or "run the deploy script": delegate to **DevOps Engineer** (e.g. by invoking this command and routing to DevOps, or by asking the user to run `./scripts/deploy.sh` themselves). Do not run `scripts/deploy.sh`, `docker compose`, or `helm install/upgrade` for Astra yourself.
