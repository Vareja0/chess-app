resource "google_compute_network" "vpc" {
  name                    = "chess-app-vpc"
  auto_create_subnetworks = false
}

resource "google_compute_subnetwork" "subnet" {
  name          = "chess-app-subnet"
  ip_cidr_range = "10.0.0.0/16"
  region        = var.region
  network       = google_compute_network.vpc.id
}

# Internal cluster traffic
resource "google_compute_firewall" "allow_internal" {
  name    = "chess-app-allow-internal"
  network = google_compute_network.vpc.name

  allow { protocol = "icmp" }
  allow { 
    protocol = "tcp" 
    ports = ["0-65535"] 
    }
  allow { 
    protocol = "udp" 
    ports = ["0-65535"] 
    }

  source_ranges = ["10.0.0.0/16"]
}

# SSH access 
resource "google_compute_firewall" "allow_ssh" {
  name    = "chess-app-allow-ssh"
  network = google_compute_network.vpc.name

  allow { 
    protocol = "tcp"
    ports = ["22"] 
   }

  source_ranges = ["0.0.0.0/0"]
}

# HTTP / HTTPS 
resource "google_compute_firewall" "allow_http_https" {
  name    = "chess-app-allow-http-https"
  network = google_compute_network.vpc.name

  allow { 
    protocol = "tcp" 
    ports = ["80", "443"] 
    }

  source_ranges = ["0.0.0.0/0"]
}

# NodePort range required by kops / kubelets
resource "google_compute_firewall" "allow_nodeports" {
  name    = "chess-app-allow-nodeports"
  network = google_compute_network.vpc.name

  allow { 
    protocol = "tcp" 
    ports = ["30000-32767"] 
    }

  source_ranges = ["0.0.0.0/0"]
}
