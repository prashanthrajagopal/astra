resource "google_sql_database_instance" "postgres" {
  name             = "astra-postgres"
  database_version = "POSTGRES_15"
  region           = var.region

  # Private IP requires the VPC peering connection to exist first
  depends_on = [google_service_networking_connection.private_vpc]

  settings {
    tier              = var.db_tier
    availability_type = "ZONAL"
    disk_size         = 10
    disk_type         = "PD_SSD"

    ip_configuration {
      ipv4_enabled    = false
      private_network = google_compute_network.vpc.id
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
