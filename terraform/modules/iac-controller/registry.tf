module "container_registry" {
  source = "../artifact-registry" # Aponta para a pasta do passo 1

  project_id           = var.project_id
  region               = var.region
  repository_name = "iac-controller"
}

locals {
  image = format("%s/%s:%s",module.container_registry.repo_url,var.sa_names["controller"], var.image_tag)
}