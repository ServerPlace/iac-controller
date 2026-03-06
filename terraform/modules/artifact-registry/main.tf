# Arquivo: modules/artifact-registry/main.tf

resource "google_artifact_registry_repository" "repo" {
  # Note que aqui usamos variáveis, pois é um módulo genérico
  project       = var.project_id
  location      = var.region
  repository_id = var.repository_name
  format        = "DOCKER"
  description   = "Gerenciado via Terraform"
}

