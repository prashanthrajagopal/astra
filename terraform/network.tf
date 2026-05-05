resource "google_compute_network" "vpc" {
  name                    = "astra-vpc"
  auto_create_subnetworks = false
  depends_on              = [google_project_service.apis["compute.googleapis.com"]]
}

resource "google_compute_subnetwork" "subnet" {
  name          = "astra-subnet"
  ip_cidr_range = "10.0.0.0/20"
  region        = var.region
  network       = google_compute_network.vpc.id
}

# Private Services Access — required for Cloud SQL private IP
resource "google_compute_global_address" "private_ip_range" {
  name          = "astra-private-ip-range"
  purpose       = "VPC_PEERING"
  address_type  = "INTERNAL"
  prefix_length = 16
  network       = google_compute_network.vpc.id
  depends_on    = [google_project_service.apis["compute.googleapis.com"]]
}

resource "google_service_networking_connection" "private_vpc" {
  network                 = google_compute_network.vpc.id
  service                 = "servicenetworking.googleapis.com"
  reserved_peering_ranges = [google_compute_global_address.private_ip_range.name]
  depends_on              = [google_project_service.apis["servicenetworking.googleapis.com"]]
}

# Serverless VPC Connector — bridges Cloud Run to private VPC resources
resource "google_vpc_access_connector" "connector" {
  name          = "astra-connector"
  region        = var.region
  ip_cidr_range = "10.8.0.0/28"
  network       = google_compute_network.vpc.name
  depends_on    = [google_project_service.apis["vpcaccess.googleapis.com"]]
}

# Cloud Router + NAT — outbound internet for Redis VM startup script and LLM calls
resource "google_compute_router" "router" {
  name    = "astra-router"
  network = google_compute_network.vpc.id
  region  = var.region
}

resource "google_compute_router_nat" "nat" {
  name                               = "astra-nat"
  router                             = google_compute_router.router.name
  region                             = var.region
  nat_ip_allocate_option             = "AUTO_ONLY"
  source_subnetwork_ip_ranges_to_nat = "ALL_SUBNETWORKS_ALL_IP_RANGES"
}

# Allow all internal VPC traffic
resource "google_compute_firewall" "internal" {
  name    = "astra-allow-internal"
  network = google_compute_network.vpc.name

  allow {
    protocol = "tcp"
    ports    = ["0-65535"]
  }
  allow {
    protocol = "udp"
    ports    = ["0-65535"]
  }
  allow {
    protocol = "icmp"
  }

  source_ranges = ["10.0.0.0/8"]
}

# IAP SSH for Redis VM maintenance
resource "google_compute_firewall" "iap_ssh" {
  name    = "astra-iap-ssh"
  network = google_compute_network.vpc.name

  allow {
    protocol = "tcp"
    ports    = ["22"]
  }

  source_ranges = ["35.235.240.0/20"]
  target_tags   = ["astra-redis"]
}
