# ===============================
# Go toolchain auto-detect (from go.mod)
# ===============================
# ===============================
# Go toolchain auto-detect (robusto, sem depender de toolchain no go.mod)
# ===============================

GO_MOD_FILE := go.mod

# Extrai "toolchain go1.xx.y" se existir (patch opcional)
GO_TOOLCHAIN_FROM_MOD := $(strip $(shell \
	if [ -f "$(GO_MOD_FILE)" ]; then \
	  awk '/^toolchain[[:space:]]+go[0-9]+\.[0-9]+(\.[0-9]+)?$$/{print $$2; exit}' "$(GO_MOD_FILE)"; \
	fi \
))

# Extrai "go 1.xx" OU "go 1.xx.y"
GO_VERSION_FROM_MOD := $(strip $(shell \
	if [ -f "$(GO_MOD_FILE)" ]; then \
	  awk '/^go[[:space:]]+[0-9]+\.[0-9]+(\.[0-9]+)?$$/{print $$2; exit}' "$(GO_MOD_FILE)"; \
	fi \
))

# Normaliza para major.minor (remove patch se existir)
GO_MAJOR_MINOR := $(strip $(shell \
	if [ -n "$(GO_VERSION_FROM_MOD)" ]; then \
	  echo "$(GO_VERSION_FROM_MOD)" | awk -F. 'NF>=2{print $$1"."$$2}'; \
	fi \
))

# Se houver toolchain no go.mod, usa ela;
# senão, se houver "go 1.xx", monta go<major>.<minor>.0;
# senão, deixa vazio (não exporta GOTOOLCHAIN)
GO_TOOLCHAIN ?= $(strip \
  $(if $(GO_TOOLCHAIN_FROM_MOD),$(GO_TOOLCHAIN_FROM_MOD), \
    $(if $(GO_MAJOR_MINOR),go$(GO_MAJOR_MINOR).0,) \
  ) \
)
GO_TOOLCHAIN := go$(GO_VERSION_FROM_MOD)

# Só exporta se tiver valor válido (evita "go.0")
ifneq ($(GO_TOOLCHAIN),)
export GOTOOLCHAIN := $(GO_TOOLCHAIN)
endif

.PHONY: go-toolchain
go-toolchain:
	@echo "toolchain(go.mod) = $(GO_TOOLCHAIN_FROM_MOD)"
	@echo "go(go.mod)        = $(GO_VERSION_FROM_MOD)"
	@echo "GO_TOOLCHAIN      = $(GO_TOOLCHAIN)"
	@echo "GOTOOLCHAIN(env)  = $(GOTOOLCHAIN)"
	@go version

# ==============================================================================
# 1. CONFIGURAÇÕES & VARIÁVEIS
# ==============================================================================
APP_NAME      := iac-controller
BUILD_DIR     := bin
CMD_PATH      := cmd/controller/main.go
VERSION := $(shell git ls-files -- '*.go' 'Dockerfile' 'go.mod' 'go.sum' | sort | git hash-object --stdin-paths | sha256sum | head -c 7)
# VERSION := $(shell git describe --tags --always --dirty)
BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
PKG := github.com/ServerPlace/iac-controller/pkg/version

# --- Detecção Automática (Igual ao seu script bash) ---
# Tenta pegar o Project ID do gcloud, ou permite override via 'make deploy GCP_PROJECT=xxx'
GCP_PROJECT   ?= $(shell gcloud config get-value project)
GCP_REGION    ?= us-central1
SERVICE_NAME  ?= iac-controller

# --- Tagging Strategy (v1-hash) ---
GIT_HASH      := $(VERSION)
# Se não tiver git, usa timestamp (fallback)
ifeq ($(GIT_HASH),)
	IMAGE_TAG := v1-$(shell date +%s)
else
	IMAGE_TAG := v1-$(GIT_HASH)
endif

# --- Artifact Registry URL ---
REPO_URL      := $(GCP_REGION)-docker.pkg.dev/$(GCP_PROJECT)/$(SERVICE_NAME)/$(SERVICE_NAME)

# --- Variáveis Complexas do Terraform ---
# Atenção: Mantive a URL hardcoded do seu script, mas o ideal seria torná-la dinâmica via Terraform output
TF_AUDIENCES  := ["https://iac-controller-803534118780.us-central1.run.app","32555940559.apps.googleusercontent.com"]


# --- Go Build Flags ---
GO_LDFLAGS := -s -w \
              -X '$(PKG).Version=$(VERSION)' \
              -X '$(PKG).BuildTime=$(BUILD_TIME)'

# ==============================================================================
# 2. TARGETS (COMANDOS)
# ==============================================================================
.PHONY: help all go-check build-local docker-auth docker-build docker-push tf-init tf-plan tf-apply deploy deploy-infra

help: ## Mostra os comandos disponíveis
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

all: go-check build-local ## Roda testes e compila localmente

# --- Desenvolvimento Go (Local) ---

go-check: ## Formata, roda linter e testes
	go mod tidy
	go fmt ./...
	go test -race ./...
	# golangci-lint run ./... (Descomente se tiver instalado)

build-local: ## Compila o binário na sua máquina (sem Docker)
	@echo "📦 Compilando $(APP_NAME) localmente..."
	go build $(GO_LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME) $(CMD_PATH)

# --- Containerização (Docker) ---

docker-auth: ## Autentica o Docker no GCP (Igual ao script)
	@echo "🔑 Configurando autenticação do Docker..."
	gcloud auth configure-docker $(GCP_REGION)-docker.pkg.dev --quiet

docker-build: ## Build da imagem com platform linux/amd64
	@echo "🐳 Construindo imagem: $(REPO_URL):$(IMAGE_TAG)"
	docker build --platform linux/amd64 --build-arg PKG="$(PKG)" --build-arg VERSION=$(VERSION) --build-arg BUILD_TIME=$(BUILD_TIME) -t "$(REPO_URL):$(IMAGE_TAG)" .

docker-push: docker-auth docker-build ## Envia a imagem para o Registry
	@echo "⬆️ Enviando imagem..."
	docker push "$(REPO_URL):$(IMAGE_TAG)"

# --- Infraestrutura (Terraform) ---

tf-init: ## Inicializa o Terraform
	terraform -chdir=terraform init

tf-apply: ## Aplica as mudanças
	@echo "🚀 Aplicando Terraform com Tag: $(IMAGE_TAG)"
	terraform -chdir=terraform apply \
		-var="project_id=$(GCP_PROJECT)" \
		-var="region=$(GCP_REGION)" \
		-var="image_tag=$(IMAGE_TAG)" \
		-var='expected_audiences=$(TF_AUDIENCES)' \
		-auto-approve

# --- Workflows de Deploy ---

deploy: docker-push tf-apply ## [Full] Build -> Push -> Terraform Apply
	@echo "✅ Deploy Completo Finalizado! Versão: $(IMAGE_TAG)"

deploy-infra: tf-apply ## [Infra Only] Pula o Docker (Equivalente a passar "TERRAFORM" no script)
	@echo "✅ Infraestrutura atualizada (Imagem mantida na versão: $(IMAGE_TAG))"
