# Deploy

Run deployment for Astra. **Only the DevOps agent** may execute deployment.

## Routing

- **Delegate to:** DevOps Engineer (do not run deploy script or Helm yourself unless you are DevOps).
- **Local:** DevOps runs `scripts/deploy.sh` from the repo root.
- **GCP:** DevOps runs `./scripts/gcp-deploy.sh` (see `README.md` GCP section; GCS workspace bucket, not MinIO).

## Steps (for DevOps agent)

1. Read `.cursor/skills/devops-deployment/SKILL.md` for the full runbook.
2. **Local deployment:** From repo root, run `./scripts/deploy.sh`. Report outcome: which services used native vs Docker, log paths, how to stop.
3. **GCP deployment:** Run `./scripts/gcp-deploy.sh` with `--setup` on first provision, then `--dev` or `--prod` for build/deploy. Document bucket name and values overrides.

## For non-DevOps agents

If the user asks you to "deploy" or "run the deploy script": delegate to **DevOps Engineer** (e.g. by invoking this command and routing to DevOps, or by asking the user to run `./scripts/deploy.sh` themselves). Do not run `scripts/deploy.sh`, `scripts/gcp-deploy.sh`, `docker compose`, or `helm install/upgrade` for Astra yourself.
