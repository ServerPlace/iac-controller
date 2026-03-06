output "repo_location" {
  description = "Localização do repositório."
  value       = google_artifact_registry_repository.repo.location
}
output "name" {
  description = "O nome do recurso no Terraform."
  value       = google_artifact_registry_repository.repo.name
}

output "repo_url" {
  description = "URL base do repositório para uso em imagens Docker (sem tag)."
  # MUDANÇA: Usando atributo nativo em vez de construir string na mão.
  # Requer provider google >= 4.0
  value       = "${var.region}-docker.pkg.dev/${var.project_id}/${var.repository_name}"
}