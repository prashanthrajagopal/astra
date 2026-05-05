# e2-micro VM running Redis — cheaper than Cloud Memorystore for dev/small deployments
resource "google_compute_instance" "redis" {
  name         = "astra-redis"
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
    network    = google_compute_network.vpc.id
    subnetwork = google_compute_subnetwork.subnet.id
    # No external IP — outbound via Cloud NAT
  }

  # Install Redis and bind to all interfaces within VPC
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

  depends_on = [google_compute_network.vpc]
}
