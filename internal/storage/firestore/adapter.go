package firestore

import (
	"context"
	"fmt"
	"github.com/ServerPlace/iac-controller/pkg/log"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/ServerPlace/iac-controller/internal/core/model"
)

// Constantes para nomes de coleções Firestore
const (
	LocksCollection        = "locks"        // Locks de stacks (antes: "semaphores")
	DeploymentsCollection  = "deployments"  // Metadados de PRs
	JobsCollection         = "jobs"         // Execuções terraform
	RepositoriesCollection = "repositories" // Repos registrados
)

type Adapter struct {
	client *firestore.Client
}

func New(client *firestore.Client) *Adapter {
	return &Adapter{client: client}
}

// ==========================================
// REPOSITÓRIOS
// ==========================================

func (a *Adapter) SaveRepository(ctx context.Context, r model.RepositoryMetadata) error {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now()
	}
	_, err := a.client.Collection(RepositoriesCollection).Doc(r.ID).Set(ctx, r)
	return err
}

// GetRepositoryByID busca repositório pelo ID nativo (chave primária)
func (a *Adapter) GetRepositoryByID(ctx context.Context, id string) (*model.RepositoryMetadata, error) {
	doc, err := a.client.Collection(RepositoriesCollection).Doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, fmt.Errorf("repository not found: %s", id)
		}
		return nil, err
	}

	var r model.RepositoryMetadata
	if err := doc.DataTo(&r); err != nil {
		return nil, err
	}
	return &r, nil
}

// GetRepositoryByName busca repositório pelo nome
// IMPORTANTE: Para uso durante migração/transição
// Considera-se legacy - preferir GetRepositoryByID
func (a *Adapter) GetRepositoryByName(ctx context.Context, name string) (*model.RepositoryMetadata, error) {
	iter := a.client.Collection(RepositoriesCollection).Where("name", "==", name).Limit(1).Documents(ctx)

	doc, err := iter.Next()
	if err == iterator.Done {
		return nil, fmt.Errorf("repository not found: %s", name)
	}
	if err != nil {
		return nil, err
	}

	var r model.RepositoryMetadata
	if err := doc.DataTo(&r); err != nil {
		return nil, err
	}
	return &r, nil
}

// GetRepositoryByURI busca repositório pela URI completa
// Útil para migração e lookup durante transição
func (a *Adapter) GetRepositoryByURI(ctx context.Context, uri string) (*model.RepositoryMetadata, error) {
	iter := a.client.Collection(RepositoriesCollection).Where("repo_uri", "==", uri).Limit(1).Documents(ctx)

	doc, err := iter.Next()
	if err == iterator.Done {
		return nil, fmt.Errorf("repository not found with URI: %s", uri)
	}
	if err != nil {
		return nil, err
	}

	var r model.RepositoryMetadata
	if err := doc.DataTo(&r); err != nil {
		return nil, err
	}
	return &r, nil
}

// ==========================================
// DEPLOYMENTS & JOBS
// ==========================================

func (a *Adapter) SaveDeployment(ctx context.Context, d *model.Deployment) error {
	_, err := a.client.Collection(DeploymentsCollection).Doc(d.ID).Set(ctx, d)
	return err
}

func (a *Adapter) GetDeployment(ctx context.Context, id string) (*model.Deployment, error) {
	doc, err := a.client.Collection(DeploymentsCollection).Doc(id).Get(ctx)
	if err != nil {
		return nil, err
	}
	var d model.Deployment
	if err := doc.DataTo(&d); err != nil {
		return nil, err
	}
	return &d, nil
}

// GetDeploymentByPR busca deployment por repo e PR number
// Usado para implementar idempotência no registro de plans
// Retorna erro se não encontrado
func (a *Adapter) GetDeploymentByPR(ctx context.Context, repoId string, prNumber int) (*model.Deployment, error) {
	logger := log.FromContext(ctx)
	// Query: WHERE repo_id == X AND pr_number == Y
	iter := a.client.Collection(DeploymentsCollection).
		Where("repo_id", "==", repoId).
		Where("pr_number", "==", prNumber).
		Limit(1).
		Documents(ctx)

	doc, err := iter.Next()
	if err == iterator.Done {
		// Não encontrado - retornar erro específico
		logger.Debug().Msgf("No deployment found for repo_id %s and  PR %d", repoId, prNumber)
		return nil, fmt.Errorf("deployment not found for repo_id=%s pr=%d", repoId, prNumber)
	}
	if err != nil {
		return nil, err
	}

	var d model.Deployment
	if err := doc.DataTo(&d); err != nil {
		return nil, err
	}

	return &d, nil
}

func (a *Adapter) SaveJob(ctx context.Context, j model.Job) error {
	_, err := a.client.Collection(JobsCollection).Doc(j.ID).Set(ctx, j)
	return err
}

func (a *Adapter) GetJob(ctx context.Context, jobID string) (*model.Job, error) {
	doc, err := a.client.Collection(JobsCollection).Doc(jobID).Get(ctx)
	if err != nil {
		return nil, err
	}
	var j model.Job
	if err := doc.DataTo(&j); err != nil {
		return nil, err
	}
	return &j, nil
}

func (a *Adapter) UpdateJobStatus(ctx context.Context, jobID string, st model.OpStatus) error {
	_, err := a.client.Collection(JobsCollection).Doc(jobID).Update(ctx, []firestore.Update{
		{Path: "status", Value: st},
		{Path: "updated_at", Value: time.Now()},
	})
	return err
}

// ==========================================
// LOCKER - SEM TTL + COLEÇÃO "locks"
// ==========================================

// AcquireBatch tenta adquirir locks para múltiplos stacks
// Locks NÃO expiram - só são liberados explicitamente
func (a *Adapter) AcquireBatch(ctx context.Context, repoID string, stacks []string, user string, prNum int) error {
	return a.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// 1. Validação: Todos os stacks disponíveis?
		for _, stack := range stacks {
			lockID := fmt.Sprintf("%s-%s", repoID, stack)
			docRef := a.client.Collection(LocksCollection).Doc(lockID)

			doc, err := tx.Get(docRef)
			if err == nil {
				// Lock existe - checar se é de outro PR
				var l model.Lock
				if err := doc.DataTo(&l); err != nil {
					return err
				}

				// Se lock é de OUTRO PR, bloqueia (sem expiração!)
				if l.PRNumber != prNum {
					return fmt.Errorf("stack '%s' locked by PR #%d (User: %s) - use /unlock to release",
						stack, l.PRNumber, l.User)
				}
				// Mesmo PR = permite re-lock (idempotente)
			} else if status.Code(err) != codes.NotFound {
				// Erro real de banco
				return err
			}
			// NotFound = livre para lock
		}

		// 2. Lock: Grava todos (all-or-nothing)
		now := time.Now()

		for _, stack := range stacks {
			lockID := fmt.Sprintf("%s-%s", repoID, stack)
			docRef := a.client.Collection(LocksCollection).Doc(lockID)

			err := tx.Set(docRef, model.Lock{
				RepoID:    repoID,
				StackPath: stack,
				PRNumber:  prNum,
				User:      user,
				CreatedAt: now, // Para auditoria - NÃO expira
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// Release libera lock de um stack específico
func (a *Adapter) Release(ctx context.Context, repoID, stackPath string) error {
	lockID := fmt.Sprintf("%s-%s", repoID, stackPath)
	_, err := a.client.Collection(LocksCollection).Doc(lockID).Delete(ctx)
	return err
}

// ReleaseBatch libera todos os locks de um PR
// Usado quando PR é merged, closed, ou comando /unlock
func (a *Adapter) ReleaseBatch(ctx context.Context, repoID string, prNum int) error {
	query := a.client.Collection(LocksCollection).
		Where("repo_id", "==", repoID).
		Where("pr_number", "==", prNum)

	docs, err := query.Documents(ctx).GetAll()
	if err != nil {
		return fmt.Errorf("failed to query locks: %w", err)
	}

	// Sem locks = não é erro (idempotente)
	if len(docs) == 0 {
		return nil
	}

	// Deletar em batch
	batch := a.client.Batch()
	for _, doc := range docs {
		batch.Delete(doc.Ref)
	}

	_, err = batch.Commit(ctx)
	if err != nil {
		return fmt.Errorf("failed to release locks: %w", err)
	}

	return nil
}
