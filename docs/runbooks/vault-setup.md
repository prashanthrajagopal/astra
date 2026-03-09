# Vault setup for Astra secrets

## Goal

Load runtime secrets from Vault in production while preserving env fallback in local development.

## Required variables

- `ASTRA_VAULT_ADDR`
- `ASTRA_VAULT_TOKEN`
- `ASTRA_VAULT_PATH` (default: `secret/data/astra`)

## Secret keys expected in Vault

- `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD`
- `REDIS_ADDR`, `MEMCACHED_ADDR`
- `ASTRA_JWT_SECRET`
- `ASTRA_TLS_ENABLED`, `ASTRA_TLS_CERT_FILE`, `ASTRA_TLS_KEY_FILE`, `ASTRA_TLS_CA_FILE`, `ASTRA_TLS_SERVER_NAME`

## Steps

1. Enable KV engine (`kv-v2` recommended) and write keys under configured path.
2. Create least-privilege token/policy with read access to the Astra path.
3. Set `ASTRA_VAULT_*` env vars in deployment manifests.
4. Restart services and confirm startup logs show successful Vault overlay.
5. Rotate token on schedule and audit access logs.

## Failure fallback

If Vault is unavailable, services continue with env/default values and emit warning logs. Treat this as degraded production posture and remediate immediately.
