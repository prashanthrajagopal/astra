locals {
  postgres_ip = google_sql_database_instance.postgres.private_ip_address
  redis_ip    = google_compute_instance.redis.network_interface[0].network_ip

  # Env vars shared by every service that connects to Postgres + Redis
  base_env = {
    POSTGRES_HOST = local.postgres_ip
    POSTGRES_PORT = "5432"
    POSTGRES_DB   = "astra"
    POSTGRES_USER = "astra"
    REDIS_ADDR    = "${local.redis_ip}:6379"
    LOG_LEVEL     = "info"
    HTTP_PORT     = "8080"
  }

  # Common Cloud Run template settings
  vpc_connector_id = google_vpc_access_connector.connector.id
  sa_email         = google_service_account.astra.email
  db_secret_id     = google_secret_manager_secret.db_password.secret_id
  jwt_secret_id    = google_secret_manager_secret.jwt_secret.secret_id
}

# ---------------------------------------------------------------------------
# Helper: reusable resource config (embedded via locals, not a module)
# ---------------------------------------------------------------------------

# ---------------------------------------------------------------------------
# identity — JWT issuance, user auth
# ---------------------------------------------------------------------------
resource "google_cloud_run_v2_service" "identity" {
  name     = "identity"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_INTERNAL_ONLY"
  depends_on = [
    google_project_service.apis["run.googleapis.com"],
    google_sql_database_instance.postgres,
    google_compute_instance.redis,
  ]

  template {
    service_account = local.sa_email

    vpc_access {
      connector = local.vpc_connector_id
      egress    = "PRIVATE_RANGES_ONLY"
    }

    scaling {
      min_instance_count = 0
      max_instance_count = 3
    }

    containers {
      image = "${local.image_base}:identity-${local.tag}"

      ports {
        container_port = 8080
      }

      dynamic "env" {
        for_each = local.base_env
        content {
          name  = env.key
          value = env.value
        }
      }

      env {
        name = "POSTGRES_PASSWORD"
        value_source {
          secret_key_ref {
            secret  = local.db_secret_id
            version = "latest"
          }
        }
      }

      env {
        name = "ASTRA_JWT_SECRET"
        value_source {
          secret_key_ref {
            secret  = local.jwt_secret_id
            version = "latest"
          }
        }
      }

      resources {
        limits = {
          cpu    = "1"
          memory = "256Mi"
        }
      }
    }
  }
}


# ---------------------------------------------------------------------------
# access-control
# ---------------------------------------------------------------------------
resource "google_cloud_run_v2_service" "access_control" {
  name     = "access-control"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_INTERNAL_ONLY"
  depends_on = [
    google_project_service.apis["run.googleapis.com"],
    google_sql_database_instance.postgres,
  ]

  template {
    service_account = local.sa_email

    vpc_access {
      connector = local.vpc_connector_id
      egress    = "PRIVATE_RANGES_ONLY"
    }

    scaling {
      min_instance_count = 0
      max_instance_count = 3
    }

    containers {
      image = "${local.image_base}:access-control-${local.tag}"

      ports {
        container_port = 8080
      }

      dynamic "env" {
        for_each = local.base_env
        content {
          name  = env.key
          value = env.value
        }
      }

      env {
        name = "POSTGRES_PASSWORD"
        value_source {
          secret_key_ref {
            secret  = local.db_secret_id
            version = "latest"
          }
        }
      }

      env {
        name = "ASTRA_JWT_SECRET"
        value_source {
          secret_key_ref {
            secret  = local.jwt_secret_id
            version = "latest"
          }
        }
      }

      resources {
        limits = {
          cpu    = "1"
          memory = "256Mi"
        }
      }
    }
  }
}


# ---------------------------------------------------------------------------
# goal-service — creates and tracks goals, runs planner
# ---------------------------------------------------------------------------
resource "google_cloud_run_v2_service" "goal_service" {
  name     = "goal-service"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_INTERNAL_ONLY"
  depends_on = [
    google_project_service.apis["run.googleapis.com"],
    google_sql_database_instance.postgres,
    google_compute_instance.redis,
  ]

  template {
    service_account = local.sa_email

    vpc_access {
      connector = local.vpc_connector_id
      egress    = "PRIVATE_RANGES_ONLY"
    }

    scaling {
      min_instance_count = 0
      max_instance_count = 3
    }

    containers {
      image = "${local.image_base}:goal-service-${local.tag}"

      ports {
        container_port = 8080
      }

      dynamic "env" {
        for_each = local.base_env
        content {
          name  = env.key
          value = env.value
        }
      }

      env {
        name = "POSTGRES_PASSWORD"
        value_source {
          secret_key_ref {
            secret  = local.db_secret_id
            version = "latest"
          }
        }
      }

      env {
        name = "ASTRA_JWT_SECRET"
        value_source {
          secret_key_ref {
            secret  = local.jwt_secret_id
            version = "latest"
          }
        }
      }

      resources {
        limits = {
          cpu    = "1"
          memory = "512Mi"
        }
      }
    }
  }
}


# ---------------------------------------------------------------------------
# worker-manager
# ---------------------------------------------------------------------------
resource "google_cloud_run_v2_service" "worker_manager" {
  name     = "worker-manager"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_INTERNAL_ONLY"
  depends_on = [
    google_project_service.apis["run.googleapis.com"],
    google_sql_database_instance.postgres,
  ]

  template {
    service_account = local.sa_email

    vpc_access {
      connector = local.vpc_connector_id
      egress    = "PRIVATE_RANGES_ONLY"
    }

    scaling {
      min_instance_count = 0
      max_instance_count = 3
    }

    containers {
      image = "${local.image_base}:worker-manager-${local.tag}"

      ports {
        container_port = 8080
      }

      dynamic "env" {
        for_each = local.base_env
        content {
          name  = env.key
          value = env.value
        }
      }

      env {
        name = "POSTGRES_PASSWORD"
        value_source {
          secret_key_ref {
            secret  = local.db_secret_id
            version = "latest"
          }
        }
      }

      env {
        name = "ASTRA_JWT_SECRET"
        value_source {
          secret_key_ref {
            secret  = local.jwt_secret_id
            version = "latest"
          }
        }
      }

      resources {
        limits = {
          cpu    = "1"
          memory = "256Mi"
        }
      }
    }
  }
}


# ---------------------------------------------------------------------------
# webhook-ingest — receives external webhook events
# ---------------------------------------------------------------------------
resource "google_cloud_run_v2_service" "webhook_ingest" {
  name     = "webhook-ingest"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_INTERNAL_ONLY"
  depends_on = [
    google_project_service.apis["run.googleapis.com"],
    google_sql_database_instance.postgres,
    google_cloud_run_v2_service.goal_service,
  ]

  template {
    service_account = local.sa_email

    vpc_access {
      connector = local.vpc_connector_id
      egress    = "PRIVATE_RANGES_ONLY"
    }

    scaling {
      min_instance_count = 0
      max_instance_count = 5
    }

    containers {
      image = "${local.image_base}:webhook-ingest-${local.tag}"

      ports {
        container_port = 8080
      }

      dynamic "env" {
        for_each = local.base_env
        content {
          name  = env.key
          value = env.value
        }
      }

      env {
        name  = "GOAL_SERVICE_ADDR"
        value = google_cloud_run_v2_service.goal_service.uri
      }

      env {
        name = "POSTGRES_PASSWORD"
        value_source {
          secret_key_ref {
            secret  = local.db_secret_id
            version = "latest"
          }
        }
      }

      env {
        name = "ASTRA_JWT_SECRET"
        value_source {
          secret_key_ref {
            secret  = local.jwt_secret_id
            version = "latest"
          }
        }
      }

      resources {
        limits = {
          cpu    = "1"
          memory = "256Mi"
        }
      }
    }
  }
}


# ---------------------------------------------------------------------------
# scheduler-service
# ---------------------------------------------------------------------------
resource "google_cloud_run_v2_service" "scheduler_service" {
  name     = "scheduler-service"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_INTERNAL_ONLY"
  depends_on = [
    google_project_service.apis["run.googleapis.com"],
    google_sql_database_instance.postgres,
  ]

  template {
    service_account = local.sa_email

    vpc_access {
      connector = local.vpc_connector_id
      egress    = "PRIVATE_RANGES_ONLY"
    }

    scaling {
      min_instance_count = 0
      max_instance_count = 2
    }

    containers {
      image = "${local.image_base}:scheduler-service-${local.tag}"

      ports {
        container_port = 8080
      }

      dynamic "env" {
        for_each = local.base_env
        content {
          name  = env.key
          value = env.value
        }
      }

      env {
        name = "POSTGRES_PASSWORD"
        value_source {
          secret_key_ref {
            secret  = local.db_secret_id
            version = "latest"
          }
        }
      }

      env {
        name = "ASTRA_JWT_SECRET"
        value_source {
          secret_key_ref {
            secret  = local.jwt_secret_id
            version = "latest"
          }
        }
      }

      resources {
        limits = {
          cpu    = "1"
          memory = "256Mi"
        }
      }
    }
  }
}


# ---------------------------------------------------------------------------
# memory-service
# ---------------------------------------------------------------------------
resource "google_cloud_run_v2_service" "memory_service" {
  name     = "memory-service"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_INTERNAL_ONLY"
  depends_on = [
    google_project_service.apis["run.googleapis.com"],
    google_sql_database_instance.postgres,
    google_compute_instance.redis,
  ]

  template {
    service_account = local.sa_email

    vpc_access {
      connector = local.vpc_connector_id
      egress    = "PRIVATE_RANGES_ONLY"
    }

    scaling {
      min_instance_count = 0
      max_instance_count = 3
    }

    containers {
      image = "${local.image_base}:memory-service-${local.tag}"

      ports {
        container_port = 8080
      }

      dynamic "env" {
        for_each = local.base_env
        content {
          name  = env.key
          value = env.value
        }
      }

      env {
        name = "POSTGRES_PASSWORD"
        value_source {
          secret_key_ref {
            secret  = local.db_secret_id
            version = "latest"
          }
        }
      }

      env {
        name = "ASTRA_JWT_SECRET"
        value_source {
          secret_key_ref {
            secret  = local.jwt_secret_id
            version = "latest"
          }
        }
      }

      resources {
        limits = {
          cpu    = "1"
          memory = "512Mi"
        }
      }
    }
  }
}


# ---------------------------------------------------------------------------
# prompt-manager
# ---------------------------------------------------------------------------
resource "google_cloud_run_v2_service" "prompt_manager" {
  name     = "prompt-manager"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_INTERNAL_ONLY"
  depends_on = [
    google_project_service.apis["run.googleapis.com"],
    google_sql_database_instance.postgres,
  ]

  template {
    service_account = local.sa_email

    vpc_access {
      connector = local.vpc_connector_id
      egress    = "PRIVATE_RANGES_ONLY"
    }

    scaling {
      min_instance_count = 0
      max_instance_count = 3
    }

    containers {
      image = "${local.image_base}:prompt-manager-${local.tag}"

      ports {
        container_port = 8080
      }

      dynamic "env" {
        for_each = local.base_env
        content {
          name  = env.key
          value = env.value
        }
      }

      env {
        name = "POSTGRES_PASSWORD"
        value_source {
          secret_key_ref {
            secret  = local.db_secret_id
            version = "latest"
          }
        }
      }

      env {
        name = "ASTRA_JWT_SECRET"
        value_source {
          secret_key_ref {
            secret  = local.jwt_secret_id
            version = "latest"
          }
        }
      }

      resources {
        limits = {
          cpu    = "1"
          memory = "256Mi"
        }
      }
    }
  }
}


# ---------------------------------------------------------------------------
# planner-service
# ---------------------------------------------------------------------------
resource "google_cloud_run_v2_service" "planner_service" {
  name     = "planner-service"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_INTERNAL_ONLY"
  depends_on = [
    google_project_service.apis["run.googleapis.com"],
    google_sql_database_instance.postgres,
  ]

  template {
    service_account = local.sa_email

    vpc_access {
      connector = local.vpc_connector_id
      egress    = "PRIVATE_RANGES_ONLY"
    }

    scaling {
      min_instance_count = 0
      max_instance_count = 3
    }

    containers {
      image = "${local.image_base}:planner-service-${local.tag}"

      ports {
        container_port = 8080
      }

      dynamic "env" {
        for_each = local.base_env
        content {
          name  = env.key
          value = env.value
        }
      }

      env {
        name = "POSTGRES_PASSWORD"
        value_source {
          secret_key_ref {
            secret  = local.db_secret_id
            version = "latest"
          }
        }
      }

      env {
        name = "ASTRA_JWT_SECRET"
        value_source {
          secret_key_ref {
            secret  = local.jwt_secret_id
            version = "latest"
          }
        }
      }

      resources {
        limits = {
          cpu    = "1"
          memory = "512Mi"
        }
      }
    }
  }
}


# ---------------------------------------------------------------------------
# evaluation-service
# ---------------------------------------------------------------------------
resource "google_cloud_run_v2_service" "evaluation_service" {
  name     = "evaluation-service"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_INTERNAL_ONLY"
  depends_on = [
    google_project_service.apis["run.googleapis.com"],
    google_sql_database_instance.postgres,
  ]

  template {
    service_account = local.sa_email

    vpc_access {
      connector = local.vpc_connector_id
      egress    = "PRIVATE_RANGES_ONLY"
    }

    scaling {
      min_instance_count = 0
      max_instance_count = 2
    }

    containers {
      image = "${local.image_base}:evaluation-service-${local.tag}"

      ports {
        container_port = 8080
      }

      dynamic "env" {
        for_each = local.base_env
        content {
          name  = env.key
          value = env.value
        }
      }

      env {
        name = "POSTGRES_PASSWORD"
        value_source {
          secret_key_ref {
            secret  = local.db_secret_id
            version = "latest"
          }
        }
      }

      env {
        name = "ASTRA_JWT_SECRET"
        value_source {
          secret_key_ref {
            secret  = local.jwt_secret_id
            version = "latest"
          }
        }
      }

      resources {
        limits = {
          cpu    = "1"
          memory = "256Mi"
        }
      }
    }
  }
}


# ---------------------------------------------------------------------------
# cost-tracker
# ---------------------------------------------------------------------------
resource "google_cloud_run_v2_service" "cost_tracker" {
  name     = "cost-tracker"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_INTERNAL_ONLY"
  depends_on = [
    google_project_service.apis["run.googleapis.com"],
    google_sql_database_instance.postgres,
  ]

  template {
    service_account = local.sa_email

    vpc_access {
      connector = local.vpc_connector_id
      egress    = "PRIVATE_RANGES_ONLY"
    }

    scaling {
      min_instance_count = 0
      max_instance_count = 2
    }

    containers {
      image = "${local.image_base}:cost-tracker-${local.tag}"

      ports {
        container_port = 8080
      }

      dynamic "env" {
        for_each = local.base_env
        content {
          name  = env.key
          value = env.value
        }
      }

      env {
        name = "POSTGRES_PASSWORD"
        value_source {
          secret_key_ref {
            secret  = local.db_secret_id
            version = "latest"
          }
        }
      }

      env {
        name = "ASTRA_JWT_SECRET"
        value_source {
          secret_key_ref {
            secret  = local.jwt_secret_id
            version = "latest"
          }
        }
      }

      resources {
        limits = {
          cpu    = "1"
          memory = "256Mi"
        }
      }
    }
  }
}


# ---------------------------------------------------------------------------
# browser-worker
# ---------------------------------------------------------------------------
resource "google_cloud_run_v2_service" "browser_worker" {
  name     = "browser-worker"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_INTERNAL_ONLY"
  depends_on = [
    google_project_service.apis["run.googleapis.com"],
    google_sql_database_instance.postgres,
  ]

  template {
    service_account = local.sa_email

    vpc_access {
      connector = local.vpc_connector_id
      egress    = "PRIVATE_RANGES_ONLY"
    }

    scaling {
      min_instance_count = 0
      max_instance_count = 2
    }

    containers {
      image = "${local.image_base}:browser-worker-${local.tag}"

      ports {
        container_port = 8080
      }

      dynamic "env" {
        for_each = local.base_env
        content {
          name  = env.key
          value = env.value
        }
      }

      env {
        name = "POSTGRES_PASSWORD"
        value_source {
          secret_key_ref {
            secret  = local.db_secret_id
            version = "latest"
          }
        }
      }

      env {
        name = "ASTRA_JWT_SECRET"
        value_source {
          secret_key_ref {
            secret  = local.jwt_secret_id
            version = "latest"
          }
        }
      }

      resources {
        limits = {
          cpu    = "2"
          memory = "1Gi"
        }
      }
    }
  }
}


# ---------------------------------------------------------------------------
# tool-runtime
# ---------------------------------------------------------------------------
resource "google_cloud_run_v2_service" "tool_runtime" {
  name     = "tool-runtime"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_INTERNAL_ONLY"
  depends_on = [
    google_project_service.apis["run.googleapis.com"],
    google_sql_database_instance.postgres,
  ]

  template {
    service_account = local.sa_email

    vpc_access {
      connector = local.vpc_connector_id
      egress    = "PRIVATE_RANGES_ONLY"
    }

    scaling {
      min_instance_count = 0
      max_instance_count = 3
    }

    containers {
      image = "${local.image_base}:tool-runtime-${local.tag}"

      ports {
        container_port = 8080
      }

      dynamic "env" {
        for_each = local.base_env
        content {
          name  = env.key
          value = env.value
        }
      }

      env {
        name = "POSTGRES_PASSWORD"
        value_source {
          secret_key_ref {
            secret  = local.db_secret_id
            version = "latest"
          }
        }
      }

      env {
        name = "ASTRA_JWT_SECRET"
        value_source {
          secret_key_ref {
            secret  = local.jwt_secret_id
            version = "latest"
          }
        }
      }

      resources {
        limits = {
          cpu    = "1"
          memory = "512Mi"
        }
      }
    }
  }
}


# ---------------------------------------------------------------------------
# astra-engine — multi-container: llm-router (main) + execution-worker (sidecar)
#
# llm-router owns the ingress port (9093 gRPC/HTTP2).
# execution-worker connects to it via localhost:9093 using insecure gRPC,
# which works because both containers share the same network namespace.
# min_instance_count=1 keeps at least one instance warm so tasks are
# picked up promptly without cold-start delay.
# ---------------------------------------------------------------------------
resource "google_cloud_run_v2_service" "astra_engine" {
  name     = "astra-engine"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_INTERNAL_ONLY"
  depends_on = [
    google_project_service.apis["run.googleapis.com"],
    google_sql_database_instance.postgres,
    google_compute_instance.redis,
  ]

  template {
    service_account = local.sa_email

    vpc_access {
      connector = local.vpc_connector_id
      egress    = "PRIVATE_RANGES_ONLY"
    }

    scaling {
      min_instance_count = 1
      max_instance_count = 3
    }

    # llm-router is the main container — it handles the Cloud Run ingress port
    containers {
      name  = "llm-router"
      image = "${local.image_base}:llm-router-${local.tag}"

      ports {
        container_port = 9093
      }

      # TCP probe — just verifies gRPC port is accepting connections
      startup_probe {
        tcp_socket {
          port = 9093
        }
        initial_delay_seconds = 5
        period_seconds        = 10
        failure_threshold     = 5
      }

      dynamic "env" {
        for_each = {
          POSTGRES_HOST = local.postgres_ip
          POSTGRES_PORT = "5432"
          POSTGRES_DB   = "astra"
          POSTGRES_USER = "astra"
          REDIS_ADDR    = "${local.redis_ip}:6379"
          LOG_LEVEL     = "info"
          LLM_GRPC_PORT = "9093"
        }
        content {
          name  = env.key
          value = env.value
        }
      }

      env {
        name = "POSTGRES_PASSWORD"
        value_source {
          secret_key_ref {
            secret  = local.db_secret_id
            version = "latest"
          }
        }
      }

      env {
        name = "ASTRA_JWT_SECRET"
        value_source {
          secret_key_ref {
            secret  = local.jwt_secret_id
            version = "latest"
          }
        }
      }

      resources {
        limits = {
          cpu    = "1"
          memory = "512Mi"
        }
      }
    }

    # execution-worker is a sidecar — no ingress port, connects to llm-router via localhost
    containers {
      name       = "execution-worker"
      image      = "${local.image_base}:execution-worker-${local.tag}"
      depends_on = ["llm-router"]

      dynamic "env" {
        for_each = {
          POSTGRES_HOST  = local.postgres_ip
          POSTGRES_PORT  = "5432"
          POSTGRES_DB    = "astra"
          POSTGRES_USER  = "astra"
          REDIS_ADDR     = "${local.redis_ip}:6379"
          LOG_LEVEL      = "info"
          LLM_GRPC_ADDR  = "localhost:9093"
        }
        content {
          name  = env.key
          value = env.value
        }
      }

      env {
        name = "POSTGRES_PASSWORD"
        value_source {
          secret_key_ref {
            secret  = local.db_secret_id
            version = "latest"
          }
        }
      }

      env {
        name = "ASTRA_JWT_SECRET"
        value_source {
          secret_key_ref {
            secret  = local.jwt_secret_id
            version = "latest"
          }
        }
      }

      resources {
        limits = {
          cpu    = "1"
          memory = "512Mi"
        }
      }
    }
  }
}

# ---------------------------------------------------------------------------
# astra-gateway — multi-container:
#   api-gateway (main, HTTP 8080)
#   task-service sidecar (gRPC 9090)
#   agent-service sidecar (gRPC 9091)
#
# api-gateway connects to task-service and agent-service via localhost,
# matching the hardcoded localhost:GRPCPort / localhost:AgentGRPCPort in main.go.
# ---------------------------------------------------------------------------
resource "google_cloud_run_v2_service" "astra_gateway" {
  name     = "astra-gateway"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_INTERNAL_ONLY"
  depends_on = [
    google_project_service.apis["run.googleapis.com"],
    google_sql_database_instance.postgres,
    google_compute_instance.redis,
    google_cloud_run_v2_service.identity,
    google_cloud_run_v2_service.access_control,
    google_cloud_run_v2_service.goal_service,
    google_cloud_run_v2_service.worker_manager,
  ]

  template {
    service_account = local.sa_email

    vpc_access {
      connector = local.vpc_connector_id
      egress    = "PRIVATE_RANGES_ONLY"
    }

    scaling {
      min_instance_count = 0
      max_instance_count = 5
    }

    # task-service sidecar — provides gRPC task management on localhost:9090
    containers {
      name  = "task-service"
      image = "${local.image_base}:task-service-${local.tag}"

      startup_probe {
        tcp_socket {
          port = 9090
        }
        initial_delay_seconds = 5
        period_seconds        = 10
        failure_threshold     = 5
      }

      dynamic "env" {
        for_each = {
          POSTGRES_HOST = local.postgres_ip
          POSTGRES_PORT = "5432"
          POSTGRES_DB   = "astra"
          POSTGRES_USER = "astra"
          REDIS_ADDR    = "${local.redis_ip}:6379"
          LOG_LEVEL     = "info"
          GRPC_PORT     = "9090"
        }
        content {
          name  = env.key
          value = env.value
        }
      }

      env {
        name = "POSTGRES_PASSWORD"
        value_source {
          secret_key_ref {
            secret  = local.db_secret_id
            version = "latest"
          }
        }
      }

      env {
        name = "ASTRA_JWT_SECRET"
        value_source {
          secret_key_ref {
            secret  = local.jwt_secret_id
            version = "latest"
          }
        }
      }

      resources {
        limits = {
          cpu    = "1"
          memory = "256Mi"
        }
      }
    }

    # agent-service sidecar — provides gRPC agent management on localhost:9091
    containers {
      name  = "agent-service"
      image = "${local.image_base}:agent-service-${local.tag}"

      startup_probe {
        tcp_socket {
          port = 9091
        }
        initial_delay_seconds = 5
        period_seconds        = 10
        failure_threshold     = 5
      }

      dynamic "env" {
        for_each = {
          POSTGRES_HOST  = local.postgres_ip
          POSTGRES_PORT  = "5432"
          POSTGRES_DB    = "astra"
          POSTGRES_USER  = "astra"
          REDIS_ADDR     = "${local.redis_ip}:6379"
          LOG_LEVEL      = "info"
          AGENT_GRPC_PORT = "9091"
        }
        content {
          name  = env.key
          value = env.value
        }
      }

      env {
        name = "POSTGRES_PASSWORD"
        value_source {
          secret_key_ref {
            secret  = local.db_secret_id
            version = "latest"
          }
        }
      }

      env {
        name = "ASTRA_JWT_SECRET"
        value_source {
          secret_key_ref {
            secret  = local.jwt_secret_id
            version = "latest"
          }
        }
      }

      resources {
        limits = {
          cpu    = "1"
          memory = "256Mi"
        }
      }
    }

    # api-gateway is the main container — handles all HTTP ingress
    containers {
      name       = "api-gateway"
      image      = "${local.image_base}:api-gateway-${local.tag}"
      depends_on = ["task-service", "agent-service"]

      ports {
        container_port = 8080
      }

      dynamic "env" {
        for_each = {
          POSTGRES_HOST      = local.postgres_ip
          POSTGRES_PORT      = "5432"
          POSTGRES_DB        = "astra"
          POSTGRES_USER      = "astra"
          REDIS_ADDR         = "${local.redis_ip}:6379"
          LOG_LEVEL          = "info"
          HTTP_PORT          = "8080"
          GRPC_PORT          = "9090"
          AGENT_GRPC_PORT    = "9091"
          IDENTITY_ADDR      = google_cloud_run_v2_service.identity.uri
          ACCESS_CONTROL_ADDR = google_cloud_run_v2_service.access_control.uri
          GOAL_SERVICE_ADDR  = google_cloud_run_v2_service.goal_service.uri
          WORKER_MANAGER_ADDR = google_cloud_run_v2_service.worker_manager.uri
        }
        content {
          name  = env.key
          value = env.value
        }
      }

      env {
        name = "POSTGRES_PASSWORD"
        value_source {
          secret_key_ref {
            secret  = local.db_secret_id
            version = "latest"
          }
        }
      }

      env {
        name = "ASTRA_JWT_SECRET"
        value_source {
          secret_key_ref {
            secret  = local.jwt_secret_id
            version = "latest"
          }
        }
      }

      resources {
        limits = {
          cpu    = "1"
          memory = "512Mi"
        }
      }
    }
  }
}

