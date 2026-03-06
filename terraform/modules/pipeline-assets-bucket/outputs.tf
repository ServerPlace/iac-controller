output "bucket_name" {
  value = google_storage_bucket.pipeline_assets_bucket.name
}

output "bucket_url" {
  value = google_storage_bucket.pipeline_assets_bucket.url
}