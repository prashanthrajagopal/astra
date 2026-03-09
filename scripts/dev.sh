#!/usr/bin/env bash
set -euo pipefail

echo "Starting infrastructure..."
docker compose up -d

echo "Waiting for postgres..."
until docker compose exec postgres pg_isready -U astra > /dev/null 2>&1; do
  sleep 1
done

echo "Running migrations..."
POSTGRES_HOST=localhost POSTGRES_USER=astra POSTGRES_DB=astra PGPASSWORD=changeme bash scripts/migrate.sh

echo "Infrastructure ready. Run services with: go run ./cmd/<service>"
