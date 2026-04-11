output "static_ip_address" {
  description = "Static IP to point your domain / assign to nginx-ingress"
  value       = google_compute_address.static_ip.address
}

output "vpc_name" {
  value = google_compute_network.vpc.name
}

output "subnet_name" {
  value = google_compute_subnetwork.subnet.name
}

output "kops_state_bucket" {
  description = "Pass this as KOPS_STATE_STORE env var"
  value       = "gs://${google_storage_bucket.kops_state.name}"
}
