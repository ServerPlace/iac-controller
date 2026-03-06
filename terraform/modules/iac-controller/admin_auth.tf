# Obtém os dados do projeto para extrair o Project Number necessário para a Brand
data "google_project" "project" {
  project_id = var.project_id
}
resource "google_project_service" "iap_api" {
  project            = var.project_id
  service            = "iap.googleapis.com"
  disable_on_destroy = false
}

resource "google_iap_brand" "project_brand" {
  support_email     = var.iap_support_email
  application_title = "Cloud IAP protected Application"
  project           = var.project_id
}

# Client ID para o Dashboard Web Administrativo

resource "google_iap_client" "admin_web" {
  display_name = "IaC-Admin-Dashboard"
  brand = google_iap_brand.project_brand.name
  depends_on = [google_project_service.iap_api]
}

