# 1. Definir o projeto (para garantir que vai para o lugar certo)
export PROJECT_ID="$(gcloud config get-value project)" # Confirme se este é o ID do seu projeto

export ADO_PAT="5LwoCETxxxxxx"

echo -n $ADO_PAT | gcloud secrets versions add iac-controller-ado-pat \
    --project="$PROJECT_ID" \
    --data-file=-

echo -n "pfPmVdJAn28hSAu7" | gcloud secrets versions add iac-controller-azure-webhook-password \
    --project=$PROJECT_ID \
    --data-file=-

gcloud secrets versions list iac-controller-github-app-id --project=$PROJECT_ID
gcloud secrets versions list iac-controller-github-private-key --project=$PROJECT_ID
