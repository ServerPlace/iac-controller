output "plan_sa_email" {
  description = "Email of the plan service account"
  value       = google_service_account.plan.email
}

output "apply_sa_email" {
  description = "Email of the apply service account"
  value       = google_service_account.apply.email
}
