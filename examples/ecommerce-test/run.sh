#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."

echo "=== Astra E-Commerce Test Runner ==="
echo ""

# Ensure services are deployed
if ! curl -s http://localhost:8080/health > /dev/null 2>&1; then
    echo "Astra services not running. Starting deploy..."
    bash scripts/deploy.sh
    sleep 3
fi

echo "Services are running."
echo ""

# Clean workspace completely to prevent npx create-next-app conflicts
WORKSPACE_ROOT="${WORKSPACE_ROOT:-$(pwd)/workspace}"
WORKSPACE_DIR="$WORKSPACE_ROOT/ecommerce-store"
echo "Cleaning workspace at $WORKSPACE_ROOT"
find "$WORKSPACE_ROOT" -mindepth 1 -maxdepth 1 -exec rm -rf {} + 2>/dev/null || true
mkdir -p "$WORKSPACE_DIR"
export WORKSPACE_ROOT

echo "Workspace: $WORKSPACE_DIR"
echo ""

# Check if Node.js is available (needed for the generated project)
if ! command -v node &> /dev/null; then
    echo "WARNING: Node.js not found. The generated e-commerce project will need Node.js to run."
    echo "Install it with: brew install node"
    echo ""
fi

# Build and run the test
echo "Building e-commerce test..."
go run ./examples/ecommerce-test/ \
    --gateway "http://localhost:8080" \
    --identity "http://localhost:8085" \
    --goal-service "http://localhost:8088" \
    --access-control "http://localhost:8086" \
    --workspace "$WORKSPACE_DIR" \
    --auto-approve \
    --poll 10s \
    --timeout 30m \
    "$@"
