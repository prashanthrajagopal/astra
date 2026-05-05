terraform {
  required_version = ">= 1.5"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
    null = {
      source  = "hashicorp/null"
      version = "~> 3.0"
    }
    time = {
      source  = "hashicorp/time"
      version = "~> 0.9"
    }
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
}

# Alias for managing resources in the Shared VPC host project.
# The identity running terraform apply must have compute.xpnAdmin
# (or equivalent) on the host project.
provider "google" {
  alias   = "host"
  project = var.host_project_id
  region  = var.region
}

locals {
  image_base = "prashanth86/astra"
  tag        = var.image_tag
}

resource "google_project_service" "apis" {
  for_each = toset([
    "run.googleapis.com",
    "sqladmin.googleapis.com",
    "compute.googleapis.com",
    "servicenetworking.googleapis.com",
    "vpcaccess.googleapis.com",
    "secretmanager.googleapis.com",
    "iam.googleapis.com",
    "cloudresourcemanager.googleapis.com",
  ])
  service            = each.key
  disable_on_destroy = false
}
