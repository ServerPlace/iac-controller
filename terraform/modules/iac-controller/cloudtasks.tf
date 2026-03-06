# Cloud Tasks queue used to delay PR merges until the triggering pipeline finishes.
# The controller enqueues a task here from ClosePlan; the task calls
# POST /internal/v1/tasks/merge-pr after merge_delay_seconds.

resource "google_cloud_tasks_queue" "async" {
  name     = "iac-controller-async"
  location = var.region
  project  = var.project_id

  retry_config {
    max_attempts       = 3
    min_backoff        = "60s"
    max_backoff        = "300s"
    max_doublings      = 2
    max_retry_duration = "0s" # unlimited total duration
  }

  depends_on = [google_project_service.apis]
}

# Allow the controller SA to enqueue tasks onto this queue.
resource "google_cloud_tasks_queue_iam_member" "controller_enqueuer" {
  name     = google_cloud_tasks_queue.async.name
  location = var.region
  project  = var.project_id
  role     = "roles/cloudtasks.enqueuer"
  member   = "serviceAccount:${google_service_account.controller.email}"
}

# When creating a Cloud Tasks HTTP task with an OIDC token, the caller must have
# iam.serviceAccounts.actAs on the SA specified — even if it's the same SA.
resource "google_service_account_iam_member" "controller_actAs_self" {
  service_account_id = google_service_account.controller.name
  role               = "roles/iam.serviceAccountUser"
  member             = "serviceAccount:${google_service_account.controller.email}"
}

# Cloud Tasks service agent needs serviceAccountTokenCreator on the OIDC SA
# to generate the OIDC token when dispatching the HTTP task.
resource "google_service_account_iam_member" "cloudtasks_agent_token_creator" {
  service_account_id = google_service_account.controller.name
  role               = "roles/iam.serviceAccountTokenCreator"
  member             = "serviceAccount:service-${data.google_project.current.number}@gcp-sa-cloudtasks.iam.gserviceaccount.com"
}

