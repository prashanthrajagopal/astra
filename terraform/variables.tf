variable "project_id" {
  description = "GCP service project ID where Astra services are deployed"
  type        = string
}

variable "host_project_id" {
  description = "GCP host project that owns the Shared VPC (vpc-host-nonprod-xz142-ib904 or vpc-host-prod-sn824-er021)"
  type        = string
}

variable "shared_vpc_name" {
  description = "Name of the Shared VPC network in the host project"
  type        = string
}

variable "shared_vpc_subnet" {
  description = "Name of the subnet in the Shared VPC allocated to this service project"
  type        = string
}

variable "environment" {
  description = "Deployment environment: nonprod or prod"
  type        = string
  default     = "nonprod"

  validation {
    condition     = contains(["nonprod", "prod"], var.environment)
    error_message = "Must be nonprod or prod."
  }
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
  description = "OpenAI API key (optional)"
  type        = string
  sensitive   = true
  default     = ""
}

variable "gemini_api_key" {
  description = "Google Gemini API key (optional)"
  type        = string
  sensitive   = true
  default     = ""
}
