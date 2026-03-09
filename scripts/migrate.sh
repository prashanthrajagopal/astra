#!/usr/bin/env bash
set -euo pipefail

DB_HOST="${POSTGRES_HOST:-localhost}"
DB_PORT="${POSTGRES_PORT:-5432}"
DB_NAME="${POSTGRES_DB:-astra}"
DB_USER="${POSTGRES_USER:-astra}"

MIGRATION_DIR="$(dirname "$0")/../migrations"

echo "Running migrations against $DB_HOST:$DB_PORT/$DB_NAME..."

for f in "$MIGRATION_DIR"/*.sql; do
  echo "Applying $(basename "$f")..."
  psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -f "$f"
done

echo "All migrations applied."
