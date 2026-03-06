data "google_storage_bucket" "existing_state" {
  name = var.tf_state_bucket_name
}

locals {
  plan_permissions  = setproduct(var.target_project_ids, var.plan_roles)
  apply_permissions = setproduct(var.target_project_ids, var.apply_roles)
}

# --- Service Accounts ---

resource "google_service_account" "plan" {
  account_id   = var.sa_names["plan"]
  display_name = "IaC Plan (Read Only)"
  project      = var.project_id
}

resource "google_service_account" "apply" {
  account_id   = var.sa_names["apply"]
  display_name = "IaC Apply (Admin)"
  project      = var.project_id
}

# --- Plan: Project-level permissions ---

resource "google_project_iam_member" "plan_viewer" {
  project = var.project_id
  role    = "roles/viewer"
  member  = "serviceAccount:${google_service_account.plan.email}"
}

resource "google_project_iam_member" "plan_permissions" {
  for_each = toset(var.plan_roles)
  project  = var.project_id
  role     = each.value
  member   = "serviceAccount:${google_service_account.plan.email}"
}

resource "google_project_iam_member" "plan_external_read" {
  for_each = {
    for pair in local.plan_permissions : "${pair[0]}-${pair[1]}" => {
      project = pair[0]
      role    = pair[1]
    }
  }

  project = each.value.project
  role    = each.value.role
  member  = "serviceAccount:${google_service_account.plan.email}"
}

# --- Plan: Storage state access ---

resource "google_storage_bucket_iam_member" "plan_state_lock" {
  bucket = data.google_storage_bucket.existing_state.name
  role   = "roles/storage.objectAdmin"
  member = "serviceAccount:${google_service_account.plan.email}"
}

# --- Plan: Organization-level permissions ---

resource "google_organization_iam_member" "plan_organization" {
  for_each = toset(var.plan_org_roles)
  org_id   = var.org_id
  role     = each.value
  member   = "serviceAccount:${google_service_account.plan.email}"
}

# --- Apply: Project-level permissions ---

resource "google_project_iam_member" "apply_roles" {
  for_each = toset(var.apply_roles)
  project  = var.project_id
  role     = each.value
  member   = "serviceAccount:${google_service_account.apply.email}"
}

resource "google_project_iam_member" "apply_external_write" {
  for_each = {
    for pair in local.apply_permissions : "${pair[0]}-${pair[1]}" => {
      project = pair[0]
      role    = pair[1]
    }
  }

  project = each.value.project
  role    = each.value.role
  member  = "serviceAccount:${google_service_account.apply.email}"
}

# --- Apply: Billing account access ---

resource "google_billing_account_iam_member" "apply_billing_account_user" {
  billing_account_id = var.billing_account_id
  role               = "roles/billing.user"
  member             = "serviceAccount:${google_service_account.apply.email}"
}

# --- Apply: Storage state access ---

resource "google_storage_bucket_iam_member" "apply_state_update" {
  bucket = data.google_storage_bucket.existing_state.name
  role   = "roles/storage.objectAdmin"
  member = "serviceAccount:${google_service_account.apply.email}"
}

# --- Apply: Organization-level permissions ---

resource "google_organization_iam_member" "apply_organization" {
  for_each = toset(var.apply_org_roles)
  org_id   = var.org_id
  role     = each.value
  member   = "serviceAccount:${google_service_account.apply.email}"
}
