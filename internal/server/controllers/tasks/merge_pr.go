package tasks

import (
	"context"
	"fmt"
	"time"

	"github.com/ServerPlace/iac-controller/internal/async"
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
		if exec.Attempt >= maxMergeAttempts {
			logger.Error().Err(err).
				Int("attempt", exec.Attempt).
				Str("deployment_id", deployment.ID).
				Int("pr_number", deployment.PRNumber).
				Msg("merge-pr: max attempts reached, giving up")
			return async.Fail(fmt.Errorf("merge failed after %d attempts: %w", exec.Attempt, err)), nil
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
