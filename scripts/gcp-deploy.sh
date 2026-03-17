#!/usr/bin/env bash
# Astra GCP deployment: GKE Autopilot + Cloud SQL + Memorystore + Cloud Storage.
# Object/artifact storage uses Google Cloud Storage (bucket on --setup), not MinIO.
# Run from repo root: ./scripts/gcp-deploy.sh [--setup] [--dev|--prod] [--build-only|--deploy-only]
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

TIER="dev"
GCP_SETUP=false
BUILD_ONLY=false
DEPLOY_ONLY=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dev)         TIER="dev" ;;
    --prod)        TIER="prod" ;;
    --setup)       GCP_SETUP=true ;;
    --build-only)  BUILD_ONLY=true ;;
    --deploy-only) DEPLOY_ONLY=true ;;
    *)             echo "Unknown flag: $1"; exit 1 ;;
  esac
  shift
done

if [[ -f .env.gcp ]]; then
  set -a
  # shellcheck source=/dev/null
  source .env.gcp
  set +a
fi

GCP_PROJECT="${GCP_PROJECT:-astra1-490021}"
GCP_REGION="${GCP_REGION:-us-central1}"
GCP_CLUSTER="${GCP_CLUSTER:-astra-cluster}"
GCP_REGISTRY="${GCP_REGISTRY:-${GCP_REGION}-docker.pkg.dev/${GCP_PROJECT}/astra-repo}"
GCS_WORKSPACE_BUCKET="${GCS_WORKSPACE_BUCKET:-${GCP_PROJECT}-astra-workspace}"
TAG="${TAG:-$(git rev-parse --short HEAD 2>/dev/null || echo latest)}"

SERVICES="api-gateway identity access-control agent-service goal-service planner-service scheduler-service task-service llm-router prompt-manager evaluation-service worker-manager execution-worker browser-worker tool-runtime memory-service cost-tracker"

echo "=== Astra GCP Deploy (${TIER}) ==="
echo "Project: $GCP_PROJECT  Region: $GCP_REGION  Cluster: $GCP_CLUSTER"
echo "Registry: $GCP_REGISTRY  Tag: $TAG"
echo "Workspace bucket (GCS): gs://${GCS_WORKSPACE_BUCKET}"
echo ""

for cmd in gcloud kubectl helm docker; do
  if ! command -v $cmd &>/dev/null; then
    echo "Error: $cmd not found. Install it and re-run."
    exit 1
  fi
done

gcloud config set project "$GCP_PROJECT" --quiet

if [[ "$GCP_SETUP" == "true" ]]; then
  echo "=== Provisioning GCP Infrastructure ==="

  echo "Creating Artifact Registry..."
  gcloud artifacts repositories create astra-repo \
    --repository-format=docker --location="$GCP_REGION" \
    --description="Astra container images" 2>/dev/null || echo "  (already exists)"

  echo "Creating GKE Autopilot cluster..."
  gcloud container clusters create-auto "$GCP_CLUSTER" \
    --region="$GCP_REGION" 2>/dev/null || echo "  (already exists)"

  echo "Creating Cloud SQL (Postgres 15 + pgvector)..."
  if [[ "$TIER" == "prod" ]]; then
    SQL_TIER="db-custom-2-8192"
    SQL_HA="--availability-type=REGIONAL"
  else
    SQL_TIER="db-f1-micro"
    SQL_HA=""
  fi
  gcloud sql instances create astra-db \
    --database-version=POSTGRES_15 --tier="$SQL_TIER" $SQL_HA \
    --region="$GCP_REGION" --storage-size=10GB \
    --network=default --no-assign-ip 2>/dev/null || echo "  (already exists)"

  gcloud sql databases create astra --instance=astra-db 2>/dev/null || echo "  (database already exists)"
  gcloud sql users create astra --instance=astra-db --password="${POSTGRES_PASSWORD:-astra}" 2>/dev/null || echo "  (user already exists)"

  echo "Creating Memorystore Redis..."
  if [[ "$TIER" == "prod" ]]; then
    REDIS_TIER="STANDARD"
    REDIS_SIZE=2
  else
    REDIS_TIER="BASIC"
    REDIS_SIZE=1
  fi
  gcloud redis instances create astra-redis \
    --size="$REDIS_SIZE" --region="$GCP_REGION" \
    --tier="$REDIS_TIER" 2>/dev/null || echo "  (already exists)"

  echo "Creating Memorystore Memcached..."
  gcloud memcache instances create astra-memcached \
    --node-count=1 --node-cpu=1 --node-memory=1024 \
    --region="$GCP_REGION" 2>/dev/null || echo "  (already exists)"

  echo "Creating Cloud Storage workspace bucket (replaces MinIO on GCP)..."
  gsutil mb -l "$GCP_REGION" "gs://${GCS_WORKSPACE_BUCKET}" 2>/dev/null || echo "  (already exists)"

  echo "Infrastructure provisioning complete."
  echo ""
fi

echo "Getting GKE credentials..."
gcloud container clusters get-credentials "$GCP_CLUSTER" --region="$GCP_REGION" --quiet

if [[ "$DEPLOY_ONLY" != "true" ]]; then
  echo ""
  echo "=== Building & Pushing Docker Images ==="
  gcloud auth configure-docker "${GCP_REGION}-docker.pkg.dev" --quiet 2>/dev/null

  for svc in $SERVICES; do
    echo "  Building $svc..."
    docker build --build-arg SERVICE="$svc" \
      -t "${GCP_REGISTRY}/astra-${svc}:${TAG}" \
      -t "${GCP_REGISTRY}/astra-${svc}:latest" \
      -f Dockerfile . --quiet
    echo "  Pushing $svc..."
    docker push "${GCP_REGISTRY}/astra-${svc}:${TAG}" --quiet
    docker push "${GCP_REGISTRY}/astra-${svc}:latest" --quiet
  done
  echo "All images pushed."
fi

if [[ "$BUILD_ONLY" == "true" ]]; then
  echo ""
  echo "=== Build complete (--build-only). Skipping deploy. ==="
  exit 0
fi

echo ""
echo "=== Running Migrations ==="
VALUES_FILE="deployments/helm/astra/values-gke-${TIER}.yaml"
if [[ ! -f "$VALUES_FILE" ]]; then
  echo "Values file not found: $VALUES_FILE"
  echo "Using default: deployments/helm/astra/values-gke.yaml"
  VALUES_FILE="deployments/helm/astra/values-gke.yaml"
fi

CLOUD_SQL_IP="${CLOUD_SQL_IP:-$(grep -A1 'postgres:' "$VALUES_FILE" | grep 'host:' | awk '{print $2}' | tr -d '"' || echo "")}"
if [[ -z "$CLOUD_SQL_IP" ]]; then
  CLOUD_SQL_IP=$(gcloud sql instances describe astra-db --format='value(ipAddresses[0].ipAddress)' 2>/dev/null || echo "")
fi
PG_PASS="${POSTGRES_PASSWORD:-$(grep 'password:' "$VALUES_FILE" | head -1 | awk '{print $2}' | tr -d '"' || echo "astra")}"

if [[ -n "$CLOUD_SQL_IP" ]]; then
  echo "Running migrations against Cloud SQL ($CLOUD_SQL_IP)..."
  for f in migrations/*.sql; do
    [[ -f "$f" ]] || continue
    echo "  $(basename "$f")"
    kubectl run "astra-migrate-$(date +%s)" --rm -i --restart=Never \
      --image=postgres:15-alpine \
      --env="PGPASSWORD=$PG_PASS" \
      -- psql -h "$CLOUD_SQL_IP" -U astra -d astra -f - < "$f" 2>/dev/null || true
  done
  echo "Migrations done."
else
  echo "WARNING: Could not determine Cloud SQL IP. Run migrations manually."
fi

echo ""
echo "=== Deploying Services via Helm ==="
for svc in $SERVICES; do
  echo "  Deploying $svc..."
  helm upgrade --install "astra-${svc}" deployments/helm/astra \
    -f "$VALUES_FILE" \
    --set service.name="$svc" \
    --set image.repository="${GCP_REGISTRY}/astra-${svc}" \
    --set image.tag="$TAG" \
    --wait --timeout=120s 2>/dev/null || echo "    WARNING: $svc deploy may have timed out"
done
echo "All services deployed."

echo ""
echo "=== Seeding Super-Admin ==="
SA_EMAIL="${ASTRA_SUPER_ADMIN_EMAIL:-admin@astra.local}"
SA_PASS="${ASTRA_SUPER_ADMIN_PASSWORD:-changeme-admin}"

kubectl wait --for=condition=ready pod -l app=astra-identity --timeout=120s 2>/dev/null || true

kubectl port-forward svc/astra-identity 18085:8085 &>/dev/null &
PF_PID=$!
sleep 3

SEED_RESP=$(curl -s -w "\n%{http_code}" -X POST "http://localhost:18085/users" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$SA_EMAIL\",\"name\":\"Super Admin\",\"password\":\"$SA_PASS\",\"is_super_admin\":true}" 2>/dev/null)
SEED_CODE=$(echo "$SEED_RESP" | tail -1)
if [[ "$SEED_CODE" == "201" || "$SEED_CODE" == "200" ]]; then
  echo "  Super-admin created: $SA_EMAIL"
else
  echo "  Super-admin: already exists or HTTP $SEED_CODE"
fi

kill $PF_PID 2>/dev/null || true

echo ""
echo "=== GCP Deploy Complete ($TIER) ==="
echo ""
kubectl get pods -l app -o wide 2>/dev/null | head -25 || true
echo ""
GATEWAY_IP=$(kubectl get svc astra-api-gateway -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || echo "<pending>")
echo "Gateway: http://${GATEWAY_IP}:8080"
echo "Dashboard: http://${GATEWAY_IP}:8080/superadmin/dashboard/"
echo "GCS workspace bucket: gs://${GCS_WORKSPACE_BUCKET} (use Workload Identity for pod access; local dev uses MinIO)"
echo ""
echo "Useful commands:"
echo "  kubectl get pods"
echo "  kubectl logs -f deployment/astra-api-gateway"
echo "  kubectl port-forward svc/astra-api-gateway 8080:8080"
