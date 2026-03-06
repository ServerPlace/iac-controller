#!/usr/bin/env bash
set -euo pipefail

PROJECT_ID="$(gcloud config get-value project)"
: "${PROJECT_ID:?gcloud project not set}"

REGION="${REGION:-us-central1}"
SERVICE_NAME="${SERVICE_NAME:-iac-controller}"
#

# 1. Defina variáveis de ambiente para facilitar
export REPO_URL="${REGION}-docker.pkg.dev/${PROJECT_ID}/${SERVICE_NAME}/${SERVICE_NAME}"

# 2. Deriva a TAG apenas dos arquivos da aplicação (Go, Dockerfile, go.mod/sum)
# Mudanças somente em terraform/ não alteram a tag e não disparam rebuild
IMAGE_TAG="v1-$(git ls-files -- '*.go' 'Dockerfile' 'go.mod' 'go.sum' | sort | git hash-object --stdin-paths | sha256sum | head -c 7)"
export IMAGE_TAG

AUTO_APPROVE="${AUTO_APPROVE:-false}"

echo ">>> Deploying version: ${IMAGE_TAG}"
if [ "${1}" != "TERRAFORM" ] && [ "${1}" != "PLAN" ]; then
    # Ref: https://cloud.google.com/artifact-registry/docs/docker/authentication#gcloud-helper
    gcloud auth configure-docker "${REGION}-docker.pkg.dev"

    # 3. Build & Push apenas se a imagem ainda não existir no registry
    if docker manifest inspect "${REPO_URL}:${IMAGE_TAG}" > /dev/null 2>&1; then
        echo ">>> Image ${IMAGE_TAG} already exists in registry, skipping build"
    else
        echo ">>> Building and pushing image ${IMAGE_TAG}"
        make docker-push
    fi
fi

cd terraform
# 4. Terraform Plan or Apply injetando a TAG
if [ "${1}" = "PLAN" ]; then
    echo ">>> Running terraform plan"
    terraform init && \
    terraform plan \
      -var="image_tag=${IMAGE_TAG}"
else
    APPROVE_FLAG=""
    if [ "${AUTO_APPROVE}" = "true" ]; then
        APPROVE_FLAG="-auto-approve"
    fi
    terraform apply \
      -var="image_tag=${IMAGE_TAG}" \
      ${APPROVE_FLAG} 
fi
