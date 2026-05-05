resource "google_sql_database_instance" "postgres" {
  name             = "astra-postgres-${var.environment}"
  database_version = "POSTGRES_15"
  region           = var.region

  # Private Services Access peering is managed by the host project.
  # Cloud SQL private IP is allocated on the shared VPC network.
  depends_on = [
    google_project_service.apis["sqladmin.googleapis.com"],
    time_sleep.iam_propagation,
  ]

  settings {
    tier              = var.db_tier
    availability_type = "ZONAL"
    disk_size         = 10
    disk_type         = "PD_SSD"

    ip_configuration {
      ipv4_enabled    = false
      private_network = data.google_compute_network.shared_vpc.id
    }

    database_flags {
      name  = "max_connections"
      value = "100"
    }

    backup_configuration {
      enabled    = true
      start_time = "03:00"
    }
  }

  deletion_protection = false
}

resource "google_sql_database" "astra" {
  name     = "astra"
  instance = google_sql_database_instance.postgres.name
}

resource "google_sql_user" "astra" {
  name     = "astra"
  instance = google_sql_database_instance.postgres.name
  password = var.db_password
}
