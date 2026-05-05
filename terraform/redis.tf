# e2-micro VM running Redis — cheaper than Cloud Memorystore for dev/small deployments.
# Attached to the shared VPC subnet; no external IP (outbound via host project's Cloud NAT).
resource "google_compute_instance" "redis" {
  name         = "astra-redis-${var.environment}"
  machine_type = var.redis_machine_type
  zone         = "${var.region}-a"
  tags         = ["astra-redis"]

  boot_disk {
    initialize_params {
      image = "debian-cloud/debian-12"
      size  = 10
      type  = "pd-standard"
    }
  }

  network_interface {
    network    = data.google_compute_network.shared_vpc.id
    subnetwork = data.google_compute_subnetwork.shared_subnet.id
    # No external IP — outbound internet via host project's Cloud NAT
  }

  metadata_startup_script = <<-SCRIPT
    #!/bin/bash
    apt-get update -y
    apt-get install -y redis-server
    sed -i 's/^bind 127.0.0.1 -::1/bind 0.0.0.0/' /etc/redis/redis.conf
    sed -i 's/^protected-mode yes/protected-mode no/' /etc/redis/redis.conf
    systemctl enable redis-server
    systemctl restart redis-server
  SCRIPT

  service_account {
    email  = google_service_account.astra.email
    scopes = ["cloud-platform"]
  }
}
