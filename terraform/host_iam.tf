# ---------------------------------------------------------------------------
# Host project IAM — run with the "host" provider alias.
# Requires the Terraform identity to have compute.xpnAdmin on the host project
# (typically granted at org level to a shared infra service account).
# ---------------------------------------------------------------------------

# Service project number — needed to construct GCP-managed service agent emails.
data "google_project" "service" {
  project_id = var.project_id
}

# Attach Astra's service project to the Shared VPC host project.
# Safe to apply repeatedly — GCP is idempotent on this resource.
resource "google_compute_shared_vpc_service_project" "astra" {
  provider        = google.host
  host_project    = var.host_project_id
  service_project = var.project_id
}

# ---------------------------------------------------------------------------
# Subnet IAM — roles/compute.networkUser on the shared subnet.
# Required by each GCP service agent that creates resources in the VPC.
# ---------------------------------------------------------------------------

# Astra's own service account (Redis VM, etc.)
resource "google_compute_subnetwork_iam_member" "astra_sa" {
  provider   = google.host
  project    = var.host_project_id
  region     = var.region
  subnetwork = data.google_compute_subnetwork.shared_subnet.name
  role       = "roles/compute.networkUser"
  member     = "serviceAccount:${google_service_account.astra.email}"

  depends_on = [google_compute_shared_vpc_service_project.astra]
}

# Cloud Run service agent — needed to attach Cloud Run to the VPC connector
resource "google_compute_subnetwork_iam_member" "cloudrun_agent" {
  provider   = google.host
  project    = var.host_project_id
  region     = var.region
  subnetwork = data.google_compute_subnetwork.shared_subnet.name
  role       = "roles/compute.networkUser"
  member     = "serviceAccount:service-${data.google_project.service.number}@serverless-robot-prod.iam.gserviceaccount.com"

  depends_on = [google_compute_shared_vpc_service_project.astra]
}

# VPC Access service agent — needed to create the serverless VPC connector
resource "google_compute_subnetwork_iam_member" "vpcaccess_agent" {
  provider   = google.host
  project    = var.host_project_id
  region     = var.region
  subnetwork = data.google_compute_subnetwork.shared_subnet.name
  role       = "roles/compute.networkUser"
  member     = "serviceAccount:service-${data.google_project.service.number}@gcp-sa-vpcaccess.iam.gserviceaccount.com"

  depends_on = [google_compute_shared_vpc_service_project.astra]
}

# Cloud SQL service agent — needed for private IP allocation in the shared VPC
resource "google_compute_subnetwork_iam_member" "cloudsql_agent" {
  provider   = google.host
  project    = var.host_project_id
  region     = var.region
  subnetwork = data.google_compute_subnetwork.shared_subnet.name
  role       = "roles/compute.networkUser"
  member     = "serviceAccount:service-${data.google_project.service.number}@gcp-sa-cloud-sql.iam.gserviceaccount.com"

  depends_on = [google_compute_shared_vpc_service_project.astra]
}

# ---------------------------------------------------------------------------
# Make the VPC connector and Cloud SQL wait for IAM to propagate first.
# Without this, connector/instance creation can fail with permission errors
# due to IAM propagation lag (~30–60s).
# ---------------------------------------------------------------------------
resource "time_sleep" "iam_propagation" {
  create_duration = "60s"

  depends_on = [
    google_compute_subnetwork_iam_member.astra_sa,
    google_compute_subnetwork_iam_member.cloudrun_agent,
    google_compute_subnetwork_iam_member.vpcaccess_agent,
    google_compute_subnetwork_iam_member.cloudsql_agent,
  ]
}
