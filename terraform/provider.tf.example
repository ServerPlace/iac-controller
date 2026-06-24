terraform {
  required_version = ">= 1.5.0"
  required_providers {
    google = { source = "hashicorp/google", version = "~> 5.0" }
  }

  # --- BACKEND REMOTO ---
  backend "gcs" {
    # Coloque aqui o NOME LITERAL do bucket que já existe (sem var.)
    bucket = "<STATE_BUCKET_NAME>"

    # Esta pasta será criada AUTOMATICAMENTE pelo Terraform dentro do bucket.
    # Ela isola o estado do controller dos outros arquivos que você já tem lá.
    prefix = "terraform/iac-controller-state"
  }
}
provider "google" {
  project = var.project_id
  region  = var.region
}
