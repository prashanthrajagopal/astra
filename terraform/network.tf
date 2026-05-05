# ---------------------------------------------------------------------------
# Shared VPC — Astra is a service project; it does NOT own the VPC.
# The host project owns the network, subnets, router, NAT, and firewall rules.
# We only reference the shared resources and create the VPC Access Connector
# in the service project so Cloud Run can reach private VPC resources.
# ---------------------------------------------------------------------------

# Read the shared VPC network from the host project
data "google_compute_network" "shared_vpc" {
  name    = var.shared_vpc_name
  project = var.host_project_id
}

# Read the subnet allocated for this service project's workloads
data "google_compute_subnetwork" "shared_subnet" {
  name    = var.shared_vpc_subnet
  region  = var.region
  project = var.host_project_id
}

# VPC Access Connector — created in the service project but attached to the
# shared VPC subnet so Cloud Run services can reach Cloud SQL and Redis.
# The host project must have already granted this service project Compute Network User
# on the subnet (done by the host project admin via Shared VPC IAM).
resource "google_vpc_access_connector" "connector" {
  name    = "astra-connector"
  region  = var.region
  project = var.project_id

  subnet {
    name       = data.google_compute_subnetwork.shared_subnet.name
    project_id = var.host_project_id
  }

  depends_on = [google_project_service.apis["vpcaccess.googleapis.com"]]
}

# NOTE: Private Services Access (for Cloud SQL private IP) must be configured
# in the HOST project — not here. The host project admin runs:
#   gcloud services vpc-peerings connect \
#     --service=servicenetworking.googleapis.com \
#     --ranges=<allocated-range> \
#     --network=<shared-vpc-name> \
#     --project=<host-project-id>
# This is a one-time operation per host project and is likely already done.
