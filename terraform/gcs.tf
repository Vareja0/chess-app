resource "google_storage_bucket" "kops_state" {
  name          = "${var.project_id}-kops-state"
  location      = var.region
  force_destroy = false

  uniform_bucket_level_access = true

  versioning {
    enabled = true
  }
}
