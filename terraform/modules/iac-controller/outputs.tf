output "service_url" { value = google_cloud_run_v2_service.default.uri }
output "bucket_name" {
  value = data.google_storage_bucket.existing_state.name
}

output "oauth_web_client_id" {
  description = "Client ID para acesso Web"
  value       = google_iap_client.admin_web.client_id
}

# IDs para a lista de AZP (Quem está chamando)
output "unique_id_invoker_plan" {
  description = "ID numérico para validar o campo azp da invoker de Plan"
  value       = google_service_account.invoker.unique_id
}

output "unique_id_invoker_apply" {
  description = "ID numérico para validar o campo azp da invoker de Apply"
  value       = google_service_account.invoker_apply.unique_id
}
