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
			h.notifyFailure(ctx, deployment, err.Error())
			return async.Fail(err), nil
		}
		if exec.Attempt >= maxMergeAttempts {
			finalErr := fmt.Errorf("merge failed after %d attempts: %w", exec.Attempt, err)
			logger.Error().Err(err).
				Int("attempt", exec.Attempt).
				Str("deployment_id", deployment.ID).
				Int("pr_number", deployment.PRNumber).
				Msg("merge-pr: max attempts reached, giving up")
			h.notifyFailure(ctx, deployment, finalErr.Error())
			return async.Fail(finalErr), nil
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

	logger.Info().
		Str("deployment_id", deployment.ID).
		Int("pr_number", deployment.PRNumber).
		Msg("merge-pr: PR merged successfully")
	return async.Done("merged"), nil
}

func (h *MergePRHandler) notifyFailure(ctx context.Context, deployment *model.Deployment, reason string) {
	_ = h.scm.SetStatus(ctx, "", deployment.RepoID, deployment.SourceBranchSHA,
		"failure", "Merge automático falhou", "")
	body := fmt.Sprintf("## ❌ Merge automático falhou\n\n**Motivo:** %s\n\n"+
		"O apply foi concluído com sucesso, mas o merge do PR não pôde ser realizado automaticamente.\n"+
		"Verifique o motivo acima e faça o merge manualmente.", reason)
	_ = h.scm.CommentUpdate(ctx, "", deployment.RepoID, deployment.PRNumber, "merge-failure", body)
}
