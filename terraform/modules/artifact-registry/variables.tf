variable "project_id" {
  description = "ID do projeto GCP onde o registro será criado."
  type        = string
}

variable "region" {
  description = "Região do Google Cloud (ex: us-central1)."
  type        = string
  default     = "us-central1"
}

variable "repository_name" {
  description = "Nome do repositório no Artifact Registry (padrão de nomenclatura: [sistema]-imagens-[tipo])."
  type        = string
}

variable "description" {
  description = "Descrição do propósito deste repositório."
  type        = string
  default     = "Repositório de Imagens Docker gerenciado via Terraform"
}

variable "format" {
  description = "Formato do repositório (DOCKER, MAVEN, NPM, etc)."
  type        = string
  default     = "DOCKER"
}