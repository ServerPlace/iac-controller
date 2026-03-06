# --- 1. Service Accounts ---
resource "google_service_account" "controller" {
  account_id   = var.sa_names["controller"]
  display_name = "IaC Controller (Runner)"
  project      = var.project_id
}

resource "google_service_account" "invoker" {
  account_id   = var.sa_names["invoker"]
  display_name = "IaC Invoker Trigger"
  project      = var.project_id
}
resource "google_service_account" "invoker_apply" {
  account_id   = "${var.sa_names["invoker"]}-apply"
  display_name = "IaC Invoker Trigger"
  project      = var.project_id
}

# --- 2. Impersonation ---
resource "google_service_account_iam_member" "impersonate_plan" {
  service_account_id = "projects/${var.project_id}/serviceAccounts/${var.plan_sa_email}"
  role               = "roles/iam.serviceAccountTokenCreator"
  member             = "serviceAccount:${google_service_account.controller.email}"
}

resource "google_service_account_iam_member" "impersonate_apply" {
  service_account_id = "projects/${var.project_id}/serviceAccounts/${var.apply_sa_email}"
  role               = "roles/iam.serviceAccountTokenCreator"
  member             = "serviceAccount:${google_service_account.controller.email}"
}

# --- 3. Invoker ---
resource "google_cloud_run_v2_service_iam_member" "invoker_access" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.default.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.invoker.email}"
}

# bind gcp service account to gke
resource "google_service_account_iam_member" "gke_invoker_sa" {
  service_account_id = "projects/${var.project_id}/serviceAccounts/${google_service_account.controller.email}"
  role               = "roles/iam.workloadIdentityUser"
  member             = "serviceAccount:${var.project_id}.svc.id.goog[azp/azp-keda]"
}

# --- 4. Storage / Firestore ---
resource "google_project_iam_member" "controller_firestore" {
  project = var.project_id
  role    = "roles/datastore.user"
  member  = "serviceAccount:${google_service_account.controller.email}"
}

# Public access needed for webhooks
resource "google_cloud_run_v2_service_iam_member" "public_access" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.default.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

resource "google_storage_bucket_iam_member" "pipeline_assets_object_admin" {
  bucket = var.pipeline_assets_bucket_name
  role   = "roles/storage.objectAdmin"
  member = "serviceAccount:${google_service_account.invoker.email}"
}
