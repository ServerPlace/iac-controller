data "google_project" "current" {
  project_id = var.project_id
}

resource "google_project_service" "secretmanager" {
  service            = "secretmanager.googleapis.com"
  disable_on_destroy = false
}

# --- Segredos GitHub Existentes ---
# resource "google_secret_manager_secret" "gh_app_id" {
#   secret_id = "iac-controller-github-app-id"
#   replication {
#     auto {}
#   }
#   depends_on = [google_project_service.secretmanager]
# }
# resource "google_secret_manager_secret" "gh_install_id" {
#   secret_id = "iac-controller-github-install-id"
#   replication {
#     auto {}
#   }
#   depends_on = [google_project_service.secretmanager]
# }
# resource "google_secret_manager_secret" "gh_private_key" {
#   secret_id = "iac-controller-github-private-key"
#   replication {
#     auto {}
#   }
#   depends_on = [google_project_service.secretmanager]
# }
# resource "google_secret_manager_secret" "gh_webhook_secret" {
#   secret_id = "iac-controller-webhook-secret"
#   replication {
#     auto {}
#   }
#   depends_on = [google_project_service.secretmanager]
# }

# --- Azure Password
resource "google_secret_manager_secret" "ado_webhook_password" {
  secret_id = "iac-controller-azure-webhook-password"
  replication {
    auto {}
  }
  depends_on = [google_project_service.secretmanager]
}

# --- JIT Key (Gerada pelo Terraform) ---
resource "random_password" "jit_secret" {
  length  = 32
  special = false
}

resource "google_secret_manager_secret" "jit_key" {
  secret_id = "iac-controller-jit-key"
  replication {
    auto {}
  }
  depends_on = [google_project_service.secretmanager]
}

resource "google_secret_manager_secret_version" "jit_key_v1" {
  secret      = google_secret_manager_secret.jit_key.id
  secret_data = random_password.jit_secret.result
}

resource "google_secret_manager_secret" "ado_pat" {
  secret_id = "iac-controller-ado-pat"
  replication { 
    auto {} 
  }
  depends_on = [google_project_service.secretmanager]
}

# --- Config YAML ---
resource "google_secret_manager_secret" "app_config_yaml" {
  secret_id = "iac-controller-config-yaml"
  replication { 
    auto {} 
  }
  depends_on = [google_project_service.secretmanager]
}


locals {
  # Concatena os AZPs: Manuais (do vars) + Dinâmicos (SAs e Web Client)
  final_allowed_azps = distinct(concat(
    var.allowed_azps,                                    # O que você passar no .tfvars (ex: o ID do CLI Desktop)
    [
      google_iap_client.admin_web.client_id,             # Gerado pelo TF
      google_service_account.invoker.unique_id,  # Unique ID da SA de Plan
      google_service_account.invoker_apply.unique_id  # Unique ID da SA de Apply
    ]
  ))

  # Concatena Audiences: Manuais + Dinâmicas (URL do Cloud Run e Web Client)
  final_expected_audiences = distinct(concat(
    var.expected_audiences,                              # O que você passar no .tfvars
    [
      "https://${var.sa_names["controller"]}-${data.google_project.project.number}.us-central1.run.app",
      google_iap_client.admin_web.client_id
    ]
  ))
}
resource "google_secret_manager_secret_version" "app_config_yaml_v1" {
  secret = google_secret_manager_secret.app_config_yaml.id

  secret_data = templatefile("${path.module}/templates/config.yaml.tftpl", {
    project_id = var.project_id
    plan_sa    = var.plan_sa_email
    apply_sa   = var.apply_sa_email

    # Security
    admin_users_json   = jsonencode(var.admin_users)
    expected_audiences_json = jsonencode(local.final_expected_audiences)
    allowed_azps_json = jsonencode(local.final_allowed_azps)
    invoker_users_json = jsonencode([google_service_account.invoker.email])

    # Azure DevOps
    ado_org_url     = var.ado_org_url
    ado_project     = var.ado_project
    ado_pipeline_id = var.ado_pipeline_id
    ado_webhook_username = var.ado_webhook_username
    ado_webhook_password_ref = "projects/${var.project_id}/secrets/${google_secret_manager_secret.ado_webhook_password.secret_id}/versions/latest"
    ado_pat_ref     = "projects/${var.project_id}/secrets/${google_secret_manager_secret.ado_pat.secret_id}/versions/latest"

    # Referências de secrets (Mantidos)
    # gh_app_id_ref      = "projects/${var.project_id}/secrets/${google_secret_manager_secret.gh_app_id.secret_id}/versions/latest"
    # gh_install_id_ref  = "projects/${var.project_id}/secrets/${google_secret_manager_secret.gh_install_id.secret_id}/versions/latest"
    # gh_private_key_ref = "projects/${var.project_id}/secrets/${google_secret_manager_secret.gh_private_key.secret_id}/versions/latest"
    # gh_webhook_ref     = "projects/${var.project_id}/secrets/${google_secret_manager_secret.gh_webhook_secret.secret_id}/versions/latest"

    # Secret HMAC
    jit_key_ref     = "projects/${var.project_id}/secrets/${google_secret_manager_secret.jit_key.secret_id}/versions/latest"

    # Cloud Tasks
    region          = var.region
    project_number  = data.google_project.current.number
    service_name    = var.sa_names["controller"]
    cloud_tasks_sa  = google_service_account.controller.email

  })
}

# --- Permissões ---
resource "google_secret_manager_secret_iam_member" "controller_access" {
  for_each = {
    # "gh_app_id"      = google_secret_manager_secret.gh_app_id.name
    # "gh_install_id"  = google_secret_manager_secret.gh_install_id.name
    # "gh_private_key" = google_secret_manager_secret.gh_private_key.name
    # "gh_webhook"     = google_secret_manager_secret.gh_webhook_secret.name
    "ado_webhook_password" = google_secret_manager_secret.ado_webhook_password.name
    "config_yaml" = google_secret_manager_secret.app_config_yaml.name
    "jit_key"     = google_secret_manager_secret.jit_key.name
    "ado_pat"     = google_secret_manager_secret.ado_pat.name
  }

  project   = data.google_project.current.number
  secret_id = each.value
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.controller.email}"
}