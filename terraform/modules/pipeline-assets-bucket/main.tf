locals {
  bucket_name = "${var.project_id}-pipeline-assets"
}

resource "google_storage_bucket" "pipeline_assets_bucket" {
  name     = local.bucket_name
  project  = var.project_id
  location = var.region

  force_destroy = false

  uniform_bucket_level_access = true

  versioning {
    enabled = true
  }
}
