output "api_gateway_url" {
  description = "Public URL for the API gateway (primary entry point)"
  value       = google_cloud_run_v2_service.astra_gateway.uri
}

output "webhook_ingest_url" {
  description = "Public URL for webhook ingestion"
  value       = google_cloud_run_v2_service.webhook_ingest.uri
}

output "identity_url" {
  description = "URL for the identity service"
  value       = google_cloud_run_v2_service.identity.uri
}

output "goal_service_url" {
  description = "URL for the goal service"
  value       = google_cloud_run_v2_service.goal_service.uri
}

output "postgres_private_ip" {
  description = "Private IP address of the Cloud SQL instance"
  value       = google_sql_database_instance.postgres.private_ip_address
}

output "redis_private_ip" {
  description = "Private IP address of the Redis VM"
  value       = google_compute_instance.redis.network_interface[0].network_ip
}

output "service_account_email" {
  description = "Service account used by all Cloud Run services"
  value       = google_service_account.astra.email
}
