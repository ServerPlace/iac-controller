package api

import (
	"github.com/ServerPlace/iac-controller/internal/core/model"
)

// ==========================================
// REPOSITORY ENDPOINTS
// ==========================================

// CreateRepositoryRequest é o payload para registrar um novo repositório
// POST /admin/repositories
type CreateRepositoryRequest struct {
	Provider   string `json:"provider"`   // azure, github, gitlab (obrigatório)
	Identifier string `json:"identifier"` // URI ou ID nativo do repositório (obrigatório)
}

// CreateRepositoryResponse retorna os dados do repositório registrado
type CreateRepositoryResponse struct {
	model.RepositoryMetadata        // embedding - todos os campos do metadata
	RepoSecret               string `json:"repo_secret,omitempty"` // K_r derivado (só no POST)
	PlanSecret               string `json:"plan_secret,omitempty"`
	ApplySecret              string `json:"apply_secret,omitempty"`
	Instruction              string `json:"instruction,omitempty"` // Instrução para o usuário
	CreatedNew               bool   `json:"created_new"`           // true se criou novo, false se já existia
}

// UpdateRepositoryRequest é o payload para atualizar metadados de um repositório
// PATCH /admin/repositories/{id}
type UpdateRepositoryRequest struct {
	KeyVersion *int `json:"key_version"` // nova versão da chave HMAC (ex: 2)
}

// ListRepositoriesResponse retorna a lista de repositórios registrados
// GET /admin/repositories
type ListRepositoriesResponse struct {
	Repositories []model.RepositoryMetadata `json:"repositories"`
	Total        int                        `json:"total"`
}

// UpdateRepositoryResponse retorna os dados atualizados e as novas chaves derivadas
type UpdateRepositoryResponse struct {
	model.RepositoryMetadata
	PlanSecret  string `json:"plan_secret,omitempty"`
	ApplySecret string `json:"apply_secret,omitempty"`
	Instruction string `json:"instruction,omitempty"`
}

// ==========================================
// ADMIN ENDPOINTS (LEGACY - deprecated)
// ==========================================

// RegisterRepoRequest é o formato antigo (deprecated)
// Mantido apenas para referência - será removido
type RegisterRepoRequest struct {
	Name     string `json:"name"` // Ex: "minha-org/infra-repo" ou URI
	Provider string `json:"provider,omitempty"`
}

// RegisterRepoResponse é o formato antigo (deprecated)
type RegisterRepoResponse struct {
	RepoID      string `json:"repo_id"`     // UUID armazenado
	RepoSecret  string `json:"repo_secret"` // K_r (Chave derivada)
	Instruction string `json:"instruction"`
}
