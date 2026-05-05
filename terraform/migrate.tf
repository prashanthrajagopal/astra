# Cloud Run Job that runs all SQL migrations in order.
# Triggered automatically by Terraform after the database is provisioned.
# Re-runs whenever image_tag changes (idempotent — SQL files use IF NOT EXISTS / CREATE OR REPLACE).
resource "google_cloud_run_v2_job" "migrate" {
  name     = "astra-migrate"
  location = var.region
  depends_on = [
    google_project_service.apis["run.googleapis.com"],
    google_sql_database_instance.postgres,
    google_sql_user.astra,
    google_vpc_access_connector.connector,
  ]

  template {
    template {
      service_account = local.sa_email

      max_retries = 1

      vpc_access {
        connector = local.vpc_connector_id
        egress    = "ALL_TRAFFIC"
      }

      containers {
        image = "${local.image_base}:migrate-${local.tag}"

        # psql reads standard PG* env vars for connection
        env {
          name  = "PGHOST"
          value = local.postgres_ip
        }
        env {
          name  = "PGPORT"
          value = "5432"
        }
        env {
          name  = "PGDATABASE"
          value = "astra"
        }
        env {
          name  = "PGUSER"
          value = "astra"
        }
        env {
          name = "PGPASSWORD"
          value_source {
            secret_key_ref {
              secret  = local.db_secret_id
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
}

# Trigger the migration job after it is created or the image tag changes.
# Requires gcloud CLI to be available on the machine running terraform apply.
resource "null_resource" "run_migrations" {
  triggers = {
    job_id    = google_cloud_run_v2_job.migrate.id
    image_tag = var.image_tag
  }

  provisioner "local-exec" {
    command = <<-CMD
      gcloud run jobs execute astra-migrate \
        --region=${var.region} \
        --project=${var.project_id} \
        --wait
    CMD
  }

  depends_on = [google_cloud_run_v2_job.migrate]
}
