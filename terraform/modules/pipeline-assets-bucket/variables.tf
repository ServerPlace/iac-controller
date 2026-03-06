variable "project_id" {
  description = "ID do projeto GCP"
  type        = string
}

variable "region" {
  description = "Região do bucket"
  type        = string
  default     = "us-central1"
}