#!/bin/sh
set -e

echo "Running Astra database migrations..."

for f in $(ls /migrations/*.sql | sort); do
  echo "  Applying $f..."
  psql -v ON_ERROR_STOP=1 -f "$f"
done

echo "All migrations applied successfully."
