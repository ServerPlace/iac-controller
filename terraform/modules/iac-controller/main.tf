resource "google_project_service" "apis" {
  for_each = toset([
    "run.googleapis.com",
    "iam.googleapis.com",
    "iamcredentials.googleapis.com",
    "firestore.googleapis.com",       # <--- ESSENCIAL
    "artifactregistry.googleapis.com", # <--- Necessário para o repo
    "cloudtasks.googleapis.com"       # <--- Async PR merge
  ])
  project            = var.project_id
  service            = each.key
  disable_on_destroy = false
}

resource "google_cloud_run_v2_service" "default" {
  name     = var.sa_names["controller"]
  location = var.region
  project  = var.project_id

  template {
    service_account = google_service_account.controller.email
    containers {
      image = local.image

      # ENVs Explícitas (Prioridade sobre o YAML)
      # Mantidas conforme seu pedido, mas usando o formato de resolução dinâmica.
      env {
        name  = "GCP_PROJECT"
        value = var.project_id
      }
      env {
        name  = "PLAN_SERVICE_ACCOUNT"
        value = var.plan_sa_email
      }
      env {
        name  = "APPLY_SERVICE_ACCOUNT"
        value = var.apply_sa_email
      }

      # Secrets git hub via ENV (Late Binding)
      # env {
      #   name  = "GITHUB_APP_ID"
      #   value = "_secret://projects/${var.project_id}/secrets/${google_secret_manager_secret.gh_app_id.secret_id}/versions/latest"
      # }
      # env {
      #   name  = "GITHUB_INSTALL_ID"
      #   value = "_secret://projects/${var.project_id}/secrets/${google_secret_manager_secret.gh_install_id.secret_id}/versions/latest"
      # }
      # env {
      #   name  = "GITHUB_PRIVATE_KEY"
      #   value = "_secret://projects/${var.project_id}/secrets/${google_secret_manager_secret.gh_private_key.secret_id}/versions/latest"
      # }
      # env {
      #   name  = "GITHUB_WEBHOOK_SECRET"
      #   value = "_secret://projects/${var.project_id}/secrets/${google_secret_manager_secret.gh_webhook_secret.secret_id}/versions/latest"
      # }
      env {
        name = "ADO_PAT"
        value = "_secret://projects/${var.project_id}/secrets/${google_secret_manager_secret.ado_pat.secret_id}/versions/latest"
      }
      env {
        name = "ADO_WEBHOOK_PASSWORD"
        value = "_secret://projects/${var.project_id}/secrets/${google_secret_manager_secret.ado_webhook_password.secret_id}/versions/latest"
      }
      env {
        name = "LOG_LEVEL"
        value = "debug"
      }

      resources {
        limits = {
          cpu    = "1000m"
          memory = "512Mi"
        }
      }
      ports {
        container_port = 8080
        name           = "http1"
      }
      startup_probe {
        failure_threshold     = 1
        initial_delay_seconds = 0
        period_seconds        = 240
        timeout_seconds       = 240
        tcp_socket {
          port = 8080
        }
      }

      # --- MONTAGEM DO ARQUIVO DE CONFIGURAÇÃO ---
      volume_mounts {
        name       = "config-vol"
        mount_path = "/app/config"
      }
    }

    volumes {
      name = "config-vol"
      secret {
        secret = google_secret_manager_secret.app_config_yaml.secret_id
        items {
          version = "latest"
          path    = "config.yaml" # Go lerá /app/config/config.yaml
        }
      }
    }
  }

  depends_on = [
    google_project_service.apis,
    google_secret_manager_secret_iam_member.controller_access
  ]
}