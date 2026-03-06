package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ServerPlace/iac-controller/internal/config"
	"github.com/ServerPlace/iac-controller/internal/core/model"
	"github.com/ServerPlace/iac-controller/internal/core/ports"
	"github.com/ServerPlace/iac-controller/internal/credentials"
	"github.com/ServerPlace/iac-controller/internal/scm"
	"github.com/ServerPlace/iac-controller/pkg/api"
	"github.com/ServerPlace/iac-controller/pkg/log"
)

// AdminController handles administrative operations
type AdminController struct {
	Config config.Config
	Repo   ports.Persistence
	SCM    scm.Client
}

// NewAdminController creates a new admin controller
func NewAdminController(cfg config.Config, repo ports.Persistence, scmClient scm.Client) *AdminController {
	return &AdminController{
		Config: cfg,
		Repo:   repo,
		SCM:    scmClient,
	}
}

// CreateRepository registra um novo repositório no sistema
// POST /admin/repositories
//
// Aceita URI ou ID nativo do repositório
// Busca metadados via API do provider (Azure/GitHub/GitLab)
// Gera e retorna K_r (repo secret) para uso no pipeline
func (c *AdminController) CreateRepository(w http.ResponseWriter, r *http.Request) {
	logger := log.FromContext(r.Context())

	// 1. Parse request
	var req api.CreateRepositoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	// 2. Validações
	if req.Provider == "" {
		http.Error(w, "provider is required", http.StatusBadRequest)
		return
	}

	if req.Identifier == "" {
		http.Error(w, "identifier is required (repository URI or ID)", http.StatusBadRequest)
		return
	}

	logger.Info().
		Str("provider", req.Provider).
		Str("identifier", req.Identifier).
		Msg("Creating repository registration")

	// 3. Busca metadados via SCM
	metadata, err := c.fetchRepositoryMetadata(r.Context(), req.Identifier, req.Provider)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to fetch repository metadata")
		http.Error(w, fmt.Sprintf("failed to fetch repository: %v", err), http.StatusBadRequest)
		return
	}

	// 4. Verifica se já existe (idempotência)
	existing, _ := c.Repo.GetRepositoryByID(r.Context(), metadata.ID)
	var createdNew bool

	if existing != nil {
		logger.Info().Str("repo_id", metadata.ID).Msg("Persistence already registered, updating metadata")
		createdNew = false

		// Atualiza metadados (pode ter mudado nome/URI)
		metadata.CreatedAt = existing.CreatedAt // preserva created_at
		metadata.UpdatedAt = time.Now()
	} else {
		logger.Info().Str("repo_id", metadata.ID).Msg("Registering new repository")
		createdNew = true
		metadata.CreatedAt = time.Now()
	}

	// 5. Salva no banco
	if err := c.Repo.SaveRepository(r.Context(), *metadata); err != nil {
		logger.Error().Err(err).Msg("Failed to save repository")
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	// 6. Deriva K_r (repo secret)
	kR, err := credentials.DeriveRepoKey(c.Config.JITSecretKey, metadata.ID)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to derive repo key")
		http.Error(w, "internal crypto error", http.StatusInternalServerError)
		return
	}
	//PlanKey
	pRk := credentials.NewDerivationParams(metadata, credentials.WithNameSpace(credentials.NSPlan))
	kPlan, err := credentials.DeriveRepoKeys(c.Config.JITSecretKey, *pRk)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to derive Plan keys")
	}
	aRk := credentials.NewDerivationParams(metadata, credentials.WithNameSpace(credentials.NSApply))
	kApply, err := credentials.DeriveRepoKeys(c.Config.JITSecretKey, *aRk)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to derive Plan keys")
	}
	// 7. Response
	resp := api.CreateRepositoryResponse{
		RepositoryMetadata: *metadata,
		RepoSecret:         kR,
		PlanSecret:         kPlan,
		ApplySecret:        kApply,
		Instruction:        "Save 'repo_secret' as IAC_REPO_KEY in your pipeline. The Controller does NOT store this key.",
		CreatedNew:         createdNew,
	}

	logger.Info().
		Str("repo_id", metadata.ID).
		Str("repo_name", metadata.Name).
		Bool("created_new", createdNew).
		Msg("Persistence registration successful")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// fetchRepositoryMetadata é um wrapper que chama o SCM apropriado
// Para Azure, o SCM já está configurado
// No futuro, pode ter factory para GitHub/GitLab baseado no provider
func (c *AdminController) fetchRepositoryMetadata(ctx context.Context, identifier, provider string) (*model.RepositoryMetadata, error) {
	// Por enquanto, usa o SCM injetado (que já é Azure)
	// TODO: No futuro, fazer factory baseado no provider para GitHub/GitLab
	return c.SCM.FetchRepositoryMetadata(ctx, identifier)
}
