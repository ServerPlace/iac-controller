variable "project_id" { type = string }
variable "region" { default = "us-central1" }
variable "image_tag" {
  type    = string
  default = "latest"
}

variable "existing_bucket_name" {
  description = "Nome do bucket existente para o tfstate"
  type        = string
}

variable "plan_sa_email" {
  description = "Email of the plan SA (output from iac-permissions stack)"
  type        = string
}

variable "apply_sa_email" {
  description = "Email of the apply SA (output from iac-permissions stack)"
  type        = string
}

variable "custom_sa_names" {
  description = "Nomes das Service Accounts"
  type        = map(string)
  default = {
    controller = "iac-controller"
    plan       = "iac-plan"
    apply      = "iac-apply"
    invoker    = "controller-invoker"
  }
}

# --- Azure DevOps Config ---
variable "ado_org_url" {
  description = "URL da Organização Azure DevOps"
  type        = string
}

variable "ado_project" {
  description = "Nome do Projeto no Azure DevOps"
  type        = string
}

variable "ado_pipeline_id" {
  description = "ID da Pipeline de Apply"
  type        = string
}
variable "ado_webhook_username" {
  description = "Password basic auth do Azure Repo"
  type        = string
  default     = "iac-webhook"
}

variable "admin_users" {
  description = "Lista de e-mails (GCP Users) autorizados a registrar repositórios"
  type        = list(string)
  default     = []
}

variable "expected_audiences" {
  description = "Allowed Audiences"
  type        = list(string)
  default     = []
}

variable "allowed_azps" {
  description = "Allowed Oauth2 AZPs"
  type        = list(string)
  default     = []
}

variable "iap_support_email" {
  type = string
   
}
