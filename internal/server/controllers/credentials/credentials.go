package credentials

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/ServerPlace/iac-controller/internal/config"
	"github.com/ServerPlace/iac-controller/internal/core/model"
	"github.com/ServerPlace/iac-controller/internal/core/ports"
	"github.com/ServerPlace/iac-controller/internal/credentials"
	"github.com/ServerPlace/iac-controller/internal/iam"
	"github.com/ServerPlace/iac-controller/internal/scm"
	"github.com/ServerPlace/iac-controller/pkg/api"
	"github.com/ServerPlace/iac-controller/pkg/log"
)

// CredentialsController handles credential requests from iac-cli
// This is the Atlantis-style controller (handler as struct method)
type CredentialsController struct {
	Config      config.Config
	Persistence ports.Persistence
	SCM         scm.Client
	IAM         *iam.Service
}

// NewCredentialsController creates a new credentials controller
func NewCredentialsController(
	cfg config.Config,
	repoStorage ports.Persistence,
	scmClient scm.Client,
	iamSvc *iam.Service,
) *CredentialsController {
	return &CredentialsController{
		Config:      cfg,
		Persistence: repoStorage,
		SCM:         scmClient,
		IAM:         iamSvc,
	}
}

// Handle processes credential requests
func (c *CredentialsController) Handle(w http.ResponseWriter, r *http.Request) {
	logger := log.FromContext(r.Context())

	// 1. Parse Body
	var req api.CredentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Err(err).Msg("Failed to parse credentials request body.")
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	contextParams, err := credentials.FindContextParams(r.Context(), c.Persistence, req.Repo, credentials.WithNameSpace(credentials.KeyNamespace(req.Mode)))
	if err != nil {
		logger.Err(err).Msg("Failed to get repo key.")
		http.Error(w, "invalid json", http.StatusUnprocessableEntity)
		return
	}
	expectedKey, err := credentials.DeriveRepoKeys(c.Config.JITSecretKey, *contextParams)
	if err != nil {
		logger.Err(err).Msg("Failed to derive repo keys.")
		http.Error(w, "invalid json", http.StatusUnprocessableEntity)
		return
	}
	valid, err := credentials.ValidateHMAC(r.Context(), expectedKey, req)
	if err != nil {
		logger.Err(err).Msg("Failed to validate HMAC")
		http.Error(w, "invalid json", http.StatusUnprocessableEntity)
		return
	}
	if !valid {
		logger.Warn().Msg("Invalid HMAC in credentials request.")
		http.Error(w, "invalid json", http.StatusUnprocessableEntity)
		return
	}
	// 2. Validação Anti-Replay (Timestamp)
	reqTime := time.Unix(req.Timestamp, 0)
	if time.Since(reqTime).Abs() > 5*time.Minute {
		logger.Error().Msg("Expired credentials request")
		http.Error(w, "request expired", http.StatusUnauthorized)
		return
	}

	// 4. Roteamento de Lógica
	if req.Mode == api.ModeApply {
		c.handleApply(w, r, req)
	} else {
		c.handlePlan(w, r)
	}
}

func (c *CredentialsController) handlePlan(w http.ResponseWriter, r *http.Request) {
	// Plan: Apenas leitura, validado pela assinatura do repo.
	c.issueToken(w, r, api.ModePlan)
}

func (c *CredentialsController) handleApply(w http.ResponseWriter, r *http.Request, req api.CredentialsRequest) {
	ctx := r.Context()
	logger := log.FromContext(ctx)

	// 1. job_id obrigatório
	if req.JobID == "" {
		logger.Warn().Msg("Apply request missing job_id.")
		http.Error(w, "missing job_id", http.StatusBadRequest)
		return
	}

	// 2. Resolve repo e busca PR — necessário para o stale check
	prNumber, err := strconv.Atoi(req.PRNumber)
	if err != nil {
		logger.Err(err).Str("pr_number", req.PRNumber).Msg("Invalid PR number format.")
		http.Error(w, "invalid PR number", http.StatusBadRequest)
		return
	}
	mRepoMeta, err := ports.ResolveManagedRepo(ctx, c.Persistence, req.Repo)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to resolve managed repo")
		http.Error(w, "invalid job", http.StatusForbidden)
		return
	}
	pr, err := c.SCM.GetPullRequest(ctx, req.Repo, mRepoMeta.ID, prNumber)
	if err != nil {
		logger.Err(err).Int("pr_number", prNumber).Msg("Failed to fetch pull request from SCM.")
		http.Error(w, "scm error", http.StatusBadGateway)
		return
	}

	// 3. Stale Check — primeiro gate, sem exceções
	// Nenhuma credencial de apply é emitida para um plano desatualizado.
	if req.SourceBranchSHA != "" && pr.HeadSHA != "" && req.SourceBranchSHA != pr.HeadSHA {
		logger.Warn().Str("cli_sha", req.SourceBranchSHA).Str("pr_sha", pr.HeadSHA).Msg("Source mismatch")
		comment := fmt.Sprintf(
			"## ❌ Apply bloqueado — plano desatualizado\n\n"+
				"O plano foi gerado para o commit `%s`, mas o HEAD atual do PR é `%s`.\n\n"+
				"Novos commits foram adicionados após o plano ter sido aprovado. "+
				"Execute `terraform plan` novamente para atualizar o plano.\n\n"+
				"_Credencial de apply negada automaticamente pelo iac-controller._",
			req.SourceBranchSHA, pr.HeadSHA,
		)
		_ = c.SCM.CommentUpdate(ctx, mRepoMeta.ID, mRepoMeta.ID, prNumber, "stale", comment)
		http.Error(w, "stale source code", http.StatusConflict)
		return
	}

	// Verify if Target Branch has changed
	branchStatus, err := c.SCM.BranchStatus(ctx, "", mRepoMeta.ID, req.SourceBranch, req.TargetBranch)
	if err != nil {
		logger.Err(err).Msg("Failed to fetch branch status.")
		http.Error(w, "Could not check target branch status", http.StatusForbidden)
		return
	}
	if branchStatus.BehindCount > 0 {
		logger.Warn().Str("source", req.SourceBranch).Str("target", req.TargetBranch).Msgf("Branch behind_count in %d. Branch %v", branchStatus.BehindCount, branchStatus)
		comment := fmt.Sprintf(
			"## ❌ Apply bloqueado — Branch desatualizada (Drift)\n\n"+
				"O plano foi gerado para o commit `%s`, mas a branch de destino (target) recebeu atualizações.\n\n"+
				"Sua branch está desatualizada em relação à master/main. Novos commits foram detectados na target e "+
				"seu PR precisa ser atualizado (merge/rebase) e um novo `terraform plan` deve ser executado.\n\n"+
				"Para fazer isso, faça o pull da branch %s na sua máquina, execute \n git rebase origin/%s; git push --force-with-lease\n\n"+
				"**Dica:** Verifique o `behindCount` nas métricas do Azure DevOps.\n\n"+
				"_Credencial de apply negada automaticamente pelo iac-controller._",
			req.SourceBranchSHA, req.SourceBranch, req.TargetBranch,
		)
		_ = c.SCM.CommentUpdate(ctx, mRepoMeta.ID, mRepoMeta.ID, prNumber, "stale", comment)
		http.Error(w, "branch behind_count", http.StatusConflict)
		return
	}

	logger.Debug().Msgf("Issuing apply token to PR: %s. SourceBranch in req is: %s, PRSourceBranch is :%s", req.PRNumber, req.SourceBranchSHA, pr.HeadSHA)

	// 4. Early return para stacks vazio ou token ausente
	if len(req.Stacks) == 0 || req.JobToken == "" {
		c.issueToken(w, r, api.ModeApply)
		return
	}

	// 5. Valida Job
	job, err := c.Persistence.GetJob(ctx, req.JobID)
	if err != nil {
		logger.Warn().Err(err).Str("job_id", req.JobID).Msg("Job lookup failed")
		http.Error(w, "invalid job", http.StatusForbidden)
		return
	}

	// 6. SEGURANÇA JIT: Valida o Token Efêmero
	if job.JITToken == "" || job.JITToken != req.JobToken {
		logger.Warn().
			Str("job_id", req.JobID).
			Str("cli_token_fragment", req.JobToken[:4]+"...").
			Msg("JIT Token mismatch: Access Denied")
		http.Error(w, "unauthorized execution source", http.StatusForbidden)
		return
	}

	// 7. Valida Status do Job
	if job.Status != model.StatusQueued && job.Status != model.StatusRunning {
		logger.Warn().Str("job_id", req.JobID).Str("status", string(job.Status)).Msg("Job is not in a runnable state.")
		http.Error(w, "job expired or finished", http.StatusForbidden)
		return
	}

	// 8. PR Aprovado?
	if !pr.IsApproved {
		logger.Warn().Int("pr_number", prNumber).Msg("PR is not approved.")
		http.Error(w, "PR not approved", http.StatusForbidden)
		return
	}

	// 9. Locking (Batch)
	user := job.User
	if user == "" {
		user = req.Repo // Fallback caso Job.User não esteja preenchido
	}
	if len(req.Stacks) > 0 {
		err := c.Persistence.AcquireBatch(ctx, mRepoMeta.ID, req.Stacks, user, prNumber)
		if err != nil {
			logger.Warn().Err(err).Msg("Lock contention")
			http.Error(w, fmt.Sprintf("Locked: %v", err), http.StatusConflict)
			return
		}
	} else {
		logger.Warn().Msg("Apply requested without specific stacks")
	}

	// 10. Sucesso: Emite Token de Escrita (Admin)
	c.issueToken(w, r, api.ModeApply)
}

func (c *CredentialsController) issueToken(w http.ResponseWriter, r *http.Request, mode string) {
	// O IAM decide qual SA usar (Plan ou Apply) baseado no mode
	token, exp, err := c.IAM.GenerateAccessToken(r.Context(), mode)
	if err != nil {
		l := log.FromContext(r.Context())
		l.Err(err).Str("mode", mode).Msg("Failed to generate IAM access token.")
		http.Error(w, "iam error", http.StatusInternalServerError)
		return
	}
	resp := api.CredentialsResponse{
		AccessToken: token,
		ExpiresAt:   exp,
		Project:     c.Config.GCPProject,
	}
	json.NewEncoder(w).Encode(resp)
}
