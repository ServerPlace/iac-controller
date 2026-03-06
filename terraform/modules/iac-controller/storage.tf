data "google_storage_bucket" "existing_state" {
  name = var.tf_state_bucket_name
}

# Banco Firestore (Native)
resource "google_firestore_database" "database" {
  project     = var.project_id
  name        = "(default)"
  location_id = var.region
  type        = "FIRESTORE_NATIVE"

  # Depende da habilitação da API no main.tf
  depends_on = [google_project_service.apis]
}