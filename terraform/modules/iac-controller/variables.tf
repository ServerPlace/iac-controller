variable "project_id" { type = string }
variable "region"     { type = string }
variable "image_tag"  { type = string }

variable "tf_state_bucket_name" {
  description = "Nome exato do bucket JÁ EXISTENTE no GCP"
  type        = string
}

variable "plan_sa_email" {
  description = "Email of the plan service account (managed by iac-permissions stack)"
  type        = string
}

variable "apply_sa_email" {
  description = "Email of the apply service account (managed by iac-permissions stack)"
  type        = string
}

variable "sa_names" {
  type = map(string)
}

# --- Azure DevOps ---
variable "ado_org_url" {
  type = string
}
variable "ado_project" {
  type = string
}
variable "ado_pipeline_id" {
  type = string
}
variable "ado_webhook_username" {
  type = string
}

variable "admin_users" {
  type = list(string)
}

variable "allowed_azps" {
  type = list(string)
}
variable "expected_audiences" {
  type = list(string)
}

variable "pipeline_assets_bucket_name" {
  description = "Nome do bucket de assets da pipeline"
  type        = string
}

variable "iap_support_email" {
  type = string
   
}