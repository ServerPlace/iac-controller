package tasks

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ServerPlace/iac-controller/internal/async"
	"github.com/ServerPlace/iac-controller/internal/core/model"
	"github.com/ServerPlace/iac-controller/internal/core/ports"
	"github.com/ServerPlace/iac-controller/internal/scm"
	"github.com/ServerPlace/iac-controller/pkg/log"
)

const maxMergeAttempts = 3
const retryDelay = 60 * time.Second

const KindMergePR = "merge-pr"

type MergePRHandler struct {
	persistence ports.Persistence
	scm         scm.Client
}

func NewMergePRHandler(persistence ports.Persistence, scmClient scm.Client) *MergePRHandler {
	return &MergePRHandler{persistence: persistence, scm: scmClient}
}

// Run executa o merge do PR via SCM. O key do ExecutionRef é o deployment.ID.
// Retorna Retry em falhas transitórias (SCM indisponível), Fail em erros permanentes.
func (h *MergePRHandler) Run(ctx context.Context, exec async.Execution) (async.Outcome, error) {
	logger := log.FromContext(ctx)

	deployment, err := h.persistence.GetDeployment(ctx, exec.Ref.Key)
	if err != nil {
		logger.Error().Err(err).Str("deployment_id", exec.Ref.Key).Msg("merge-pr: deployment not found")
		return async.Fail(fmt.Errorf("deployment not found: %w", err)), nil
	}

	if err := h.scm.MergePR(ctx, "", deployment.RepoID, deployment.PRNumber, deployment.SourceBranchSHA); err != nil {
		var mergeErr *scm.MergeError
		if errors.As(err, &mergeErr) && !mergeErr.Retryable {
			logger.Error().Err(err).
				Str("deployment_id", deployment.ID).
				Int("pr_number", deployment.PRNumber).
				Msg("merge-pr: permanent failure, not retrying")
			return h.failWithNotify(ctx, deployment, err)
		}
		if exec.Attempt >= maxMergeAttempts {
			finalErr := fmt.Errorf("merge failed after %d attempts: %w", exec.Attempt, err)
			logger.Error().Err(err).
				Int("attempt", exec.Attempt).
				Str("deployment_id", deployment.ID).
				Int("pr_number", deployment.PRNumber).
				Msg("merge-pr: max attempts reached, giving up")
			return h.failWithNotify(ctx, deployment, finalErr)
		}
		logger.Warn().Err(err).
			Int("attempt", exec.Attempt).
			Int("max_attempts", maxMergeAttempts).
			Str("deployment_id", deployment.ID).
			Int("pr_number", deployment.PRNumber).
			Dur("retry_in", retryDelay).
			Msg("merge-pr: SCM merge failed, will retry")
		return async.Wait(retryDelay, err.Error(), ""), nil
	}

	if err := h.persistence.ReleaseBatch(ctx, deployment.RepoID, deployment.PRNumber); err != nil {
		logger.Error().Err(err).Int("pr_number", deployment.PRNumber).Msg("merge-pr: failed to release locks after merge")
	}
	logger.Info().
		Str("deployment_id", deployment.ID).
		Int("pr_number", deployment.PRNumber).
		Msg("merge-pr: PR merged and locks released")
	return async.Done("merged"), nil
}

// failWithNotify posts a failure status and PR comment before returning Fail.
// Use this for all permanent failures where the deployment context is available,
// so the user always knows the merge didn't happen.
func (h *MergePRHandler) failWithNotify(ctx context.Context, deployment *model.Deployment, err error) (async.Outcome, error) {
	_ = h.scm.SetStatus(ctx, "", deployment.RepoID, deployment.SourceBranchSHA,
		"failure", "Merge automático falhou", "")
	body := fmt.Sprintf("## ❌ Merge automático falhou\n\n**Motivo:** %s\n\n"+
		"O apply foi concluído com sucesso, mas o merge do PR não pôde ser realizado automaticamente.\n"+
		"Verifique o motivo acima e faça o merge manualmente.", err.Error())
	_ = h.scm.CommentUpdate(ctx, "", deployment.RepoID, deployment.PRNumber, "merge-failure", body)
	return async.Fail(err), nil
}
