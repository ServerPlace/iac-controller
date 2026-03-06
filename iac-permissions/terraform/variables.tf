variable "project_id" {
  description = "GCP project ID where SAs will be created"
  type        = string
}

variable "org_id" {
  description = "GCP Organization ID"
  type        = string
}

variable "billing_account_id" {
  description = "GCP Billing Account ID"
  type        = string
}

variable "region" {
  description = "GCP region"
  type        = string
  default     = "us-central1"
}

variable "tf_state_bucket_name" {
  description = "Name of the existing GCS bucket used for Terraform state"
  type        = string
}

variable "sa_names" {
  description = "Names for the plan and apply service accounts"
  type        = map(string)
  default = {
    plan  = "iac-plan"
    apply = "iac-apply"
  }
}

variable "plan_roles" {
  description = "Project-level roles for the plan SA"
  type        = list(string)
  default = [
    "roles/cloudkms.cryptoKeyDecrypter",
    "roles/cloudkms.viewer",
    "roles/compute.networkViewer",
    "roles/iam.securityReviewer",
    "roles/serviceusage.serviceUsageConsumer",
    "roles/viewer",
  ]
}

variable "apply_roles" {
  description = "Project-level roles for the apply SA"
  type        = list(string)
  default = [
    "roles/cloudkms.cryptoKeyEncrypterDecrypter",
    "roles/compute.admin",
    "roles/container.admin",
    "roles/iam.serviceAccountUser",
    "roles/resourcemanager.projectIamAdmin",
    "roles/secretmanager.admin",
    "roles/serviceusage.serviceUsageConsumer",
    "roles/storage.admin",
  ]
}

variable "plan_org_roles" {
  description = "Organization-level roles for the plan SA"
  type        = list(string)
  default = [
    "roles/billing.user",
    "roles/resourcemanager.organizationViewer",
    "roles/resourcemanager.folderViewer",
    "roles/orgpolicy.policyViewer",
    "roles/serviceusage.serviceUsageConsumer",
    "roles/compute.networkUser",
    "roles/iam.serviceAccountUser",
  ]
}

variable "apply_org_roles" {
  description = "Organization-level roles for the apply SA"
  type        = list(string)
  default = [
    "roles/billing.admin",
    "roles/compute.admin",
    "roles/compute.networkAdmin",
    "roles/compute.xpnAdmin",
    "roles/iam.securityAdmin",
    "roles/iam.serviceAccountAdmin",
    "roles/logging.configWriter",
    "roles/orgpolicy.policyAdmin",
    "roles/resourcemanager.folderAdmin",
    "roles/resourcemanager.organizationViewer",
    "roles/resourcemanager.projectCreator",
    "roles/resourcemanager.projectDeleter",
    "roles/resourcemanager.projectIamAdmin",
    "roles/secretmanager.admin",
    "roles/servicenetworking.networksAdmin",
    "roles/serviceusage.serviceUsageAdmin",
    "roles/storage.admin",
  ]
}

variable "target_project_ids" {
  description = "List of project IDs where plan/apply SAs need cross-project permissions"
  type        = list(string)
  default     = []
}
