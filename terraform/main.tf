data "google_project" "project" {
  project_id = var.project_id
}
module "iac_stack" {
  source = "./modules/iac-controller"

  project_id           = var.project_id
  region               = var.region
  image_tag            = var.image_tag
  tf_state_bucket_name = var.existing_bucket_name
  sa_names             = var.custom_sa_names
  iap_support_email = var.iap_support_email

  plan_sa_email  = var.plan_sa_email
  apply_sa_email = var.apply_sa_email

  # Azure Config
  ado_org_url          = var.ado_org_url
  ado_project          = var.ado_project
  ado_pipeline_id      = var.ado_pipeline_id
  ado_webhook_username = var.ado_webhook_username

  # Admins
  admin_users        = var.admin_users
  expected_audiences = var.expected_audiences
  allowed_azps       = var.allowed_azps
  
  # Bucket de recursos da pipeline
  pipeline_assets_bucket_name = module.pipeline_assets_bucket.bucket_name
}

module "pipeline_assets_bucket" {
  source = "./modules/pipeline-assets-bucket"

  project_id           = var.project_id
  region               = var.region
}
