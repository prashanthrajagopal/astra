variable "project_id" {
  description = "GCP project ID"
  type        = string
}

variable "region" {
  description = "GCP region for all resources"
  type        = string
  default     = "us-central1"
}

variable "image_tag" {
  description = "Docker image tag to deploy (e.g. v0.8.0)"
  type        = string
  default     = "latest"
}

variable "db_password" {
  description = "PostgreSQL password for the astra user"
  type        = string
  sensitive   = true
}

variable "jwt_secret" {
  description = "JWT signing secret for Astra"
  type        = string
  sensitive   = true
}

variable "db_tier" {
  description = "Cloud SQL instance tier"
  type        = string
  default     = "db-f1-micro"
}

variable "redis_machine_type" {
  description = "Compute Engine machine type for Redis VM"
  type        = string
  default     = "e2-micro"
}

variable "openai_api_key" {
  description = "OpenAI API key (optional, configure providers via API after deploy)"
  type        = string
  sensitive   = true
  default     = ""
}

variable "gemini_api_key" {
  description = "Google Gemini API key (optional, configure providers via API after deploy)"
  type        = string
  sensitive   = true
  default     = ""
}
