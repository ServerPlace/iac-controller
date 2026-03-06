package ports

import (
	"context"
	"github.com/ServerPlace/iac-controller/internal/core/model"
)

// Persistence gerencia persistência de dados e locks de stacks no Firestore
//
//go:generate mockgen -destination=mock_repository.go -package=ports -mock_names Persistence=MockRepository github.com/ServerPlace/iac-controller/internal/core/ports Persistence
type Persistence interface {
	// ==========================================
	// DEPLOYMENTS & JOBS
	// ==========================================

	SaveDeployment(ctx context.Context, d *model.Deployment) error
	GetDeployment(ctx context.Context, id string) (*model.Deployment, error)
	GetDeploymentByPR(ctx context.Context, repo string, prNumber int) (*model.Deployment, error)

	SaveJob(ctx context.Context, j model.Job) error
	GetJob(ctx context.Context, jobID string) (*model.Job, error)
	UpdateJobStatus(ctx context.Context, jobID string, status model.OpStatus) error

	// ==========================================
	// REPOSITÓRIOS
	// ==========================================

	// SaveRepository cria ou atualiza um repositório
	SaveRepository(ctx context.Context, r model.RepositoryMetadata) error

	// GetRepositoryByID busca repositório pelo ID nativo (chave primária)
	// ID = GUID do Azure, número do GitHub/GitLab
	GetRepositoryByID(ctx context.Context, id string) (*model.RepositoryMetadata, error)

	// GetRepositoryByName busca repositório pelo nome (busca secundária)
	// Pode retornar erro se houver múltiplos repos com mesmo nome
	GetRepositoryByName(ctx context.Context, name string) (*model.RepositoryMetadata, error)

	// GetRepositoryByURI busca repositório pela URI completa
	// Útil para migração e lookup durante transição
	GetRepositoryByURI(ctx context.Context, uri string) (*model.RepositoryMetadata, error)

	// ==========================================
	// LOCKS
	// ==========================================

	// AcquireBatch tenta adquirir locks para múltiplos stacks de um PR
	// Retorna erro se QUALQUER stack já estiver locked por outro PR
	// Se o mesmo PR chamar novamente, permite (re-lock/idempotente)
	AcquireBatch(ctx context.Context, repo string, stacks []string, user string, prNum int) error

	// Release libera o lock de um stack específico
	Release(ctx context.Context, repo, stackPath string) error

	// ReleaseBatch libera todos os locks de um PR específico
	// Usado quando PR é merged, closed, ou via comando /unlock
	ReleaseBatch(ctx context.Context, repo string, prNum int) error
}

// PipelineOrchestrator: Azure DevOps ou GitHub Actions
type PipelineOrchestrator interface {
	TriggerApply(ctx context.Context, req model.PipelineTriggerRequest) (string, error)
}
