resource "google_compute_address" "static_ip" {
  name   = "chess-app-static-ip"
  region = var.region
}
