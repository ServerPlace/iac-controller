
output "url" {
  value = {
    service_url = module.iac_stack.service_url
    endpoint = "https://${var.custom_sa_names["controller"]}-${data.google_project.project.number}.us-central1.run.app"
  }
}
output "state_bucket" {
  value = module.iac_stack.bucket_name
}
output "admin_web_id" {
  value = module.iac_stack.oauth_web_client_id
}

output "authorized_azps" {
  value = {
    admin_web = module.iac_stack.oauth_web_client_id
    invoker_plan   = module.iac_stack.unique_id_invoker_plan
    invoker_apply  = module.iac_stack.unique_id_invoker_apply
  }
}
