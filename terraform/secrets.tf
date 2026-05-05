resource "google_secret_manager_secret" "db_password" {
  secret_id  = "astra-db-password"
  depends_on = [google_project_service.apis["secretmanager.googleapis.com"]]

  replication {
    auto {}
  }
}

resource "google_secret_manager_secret_version" "db_password" {
  secret      = google_secret_manager_secret.db_password.id
  secret_data = var.db_password
}

resource "google_secret_manager_secret" "jwt_secret" {
  secret_id  = "astra-jwt-secret"
  depends_on = [google_project_service.apis["secretmanager.googleapis.com"]]

  replication {
    auto {}
  }
}

resource "google_secret_manager_secret_version" "jwt_secret" {
  secret      = google_secret_manager_secret.jwt_secret.id
  secret_data = var.jwt_secret
}

resource "google_secret_manager_secret" "openai_api_key" {
  count      = var.openai_api_key != "" ? 1 : 0
  secret_id  = "astra-openai-api-key"
  depends_on = [google_project_service.apis["secretmanager.googleapis.com"]]

  replication {
    auto {}
  }
}

resource "google_secret_manager_secret_version" "openai_api_key" {
  count       = var.openai_api_key != "" ? 1 : 0
  secret      = google_secret_manager_secret.openai_api_key[0].id
  secret_data = var.openai_api_key
}

resource "google_secret_manager_secret" "gemini_api_key" {
  count      = var.gemini_api_key != "" ? 1 : 0
  secret_id  = "astra-gemini-api-key"
  depends_on = [google_project_service.apis["secretmanager.googleapis.com"]]

  replication {
    auto {}
  }
}

resource "google_secret_manager_secret_version" "gemini_api_key" {
  count       = var.gemini_api_key != "" ? 1 : 0
  secret      = google_secret_manager_secret.gemini_api_key[0].id
  secret_data = var.gemini_api_key
}
