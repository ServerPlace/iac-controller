package plans

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/ServerPlace/iac-controller/internal/async"
	"github.com/ServerPlace/iac-controller/internal/config"
	"github.com/ServerPlace/iac-controller/internal/core/model"
	"github.com/ServerPlace/iac-controller/internal/core/ports"
	"github.com/ServerPlace/iac-controller/internal/middleware"
	"github.com/ServerPlace/iac-controller/internal/scm"
	"github.com/ServerPlace/iac-controller/pkg/api"
	"github.com/ServerPlace/iac-controller/pkg/httputil"
	"github.com/ServerPlace/iac-controller/pkg/log"
	"github.com/google/uuid"
)

var (
	ErrSourceBranchChanged = errors.New("source branch changed")
)

// PlansController handles plan registration from CLI
// When CLI runs terraform plan successfully, it calls this endpoint
type PlansController struct {
	Persistence ports.Persistence
	Config      config.Config
	SCM         scm.Client
	AsyncEngine *async.Engine
}

// NewPlansController creates a new plans controller
func NewPlansController(persistence ports.Persistence, config config.Config, scmClient scm.Client, engine *async.Engine) *PlansController {
	return &PlansController{
		Persistence: persistence,
		Config:      config,
		SCM:         scmClient,
		AsyncEngine: engine,
	}
}

// ClosePlan marca o deployment como fechado, faz merge do PR e libera os locks
// POST /api/v1/plans/close
func (c *PlansController) ClosePlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.FromContext(ctx)

	// 1. Parse request
	var req api.ClosePlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Err(err).Msg("Failed to parse close plan request body.")
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	// 2. Validações básicas
	if req.Repo == "" || req.PRNumber == 0 || req.HeadSHA == "" {
		logger.Warn().
			Str("repo", req.Repo).
			Int("pr_number", req.PRNumber).
			Str("head_sha", req.HeadSHA).
			Msg("Missing required fields in close plan request.")
		http.Error(w, "missing required fields", http.StatusBadRequest)
		return
	}

	repo := middleware.RepoFrom(ctx)

	// 3. Buscar deployment existente
	deployment, err := c.Persistence.GetDeploymentByPR(ctx, repo.ID, req.PRNumber)
	if err != nil {
		logger.Err(err).Msg("Deployment not found.")
		http.Error(w, "deployment not found", http.StatusNotFound)
		return
	}

	// 4. Libera locks do PR
	if err := c.Persistence.ReleaseBatch(ctx, repo.ID, req.PRNumber); err != nil {
		// Log mas não falha — PR já foi merged
		logger.Error().Err(err).Int("pr_number", req.PRNumber).Msg("Failed to release locks after merge")
	}

	// 5. Atualiza status do deployment para "closed"
	deployment.Status = model.DeploymentApplied
	if err := c.Persistence.SaveDeployment(ctx, deployment); err != nil {
		logger.Error().Err(err).Str("deployment_id", deployment.ID).Msg("Failed to update deployment status")
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	logger.Info().
		Str("deployment_id", deployment.ID).
		Int("pr_number", req.PRNumber).
		Msg("Plan closed, locks released")

	// 6. Enfileira merge do PR de forma assíncrona via Cloud Tasks
	if c.AsyncEngine != nil {
		delay := time.Duration(c.Config.CloudTasks.MergeDelaySeconds) * time.Second
		logger.Info().Dur("delay", delay).Msg("async: enqueueing merge-pr")
		if err := c.AsyncEngine.Kick(ctx, "merge-pr", deployment.ID, false, delay); err != nil {
			logger.Error().Err(err).Str("deployment_id", deployment.ID).Msg("Failed to enqueue merge-pr task")
		}
	}

	httputil.RespondJSON(w, http.StatusOK, api.ClosePlanResponse{
		DeploymentID: deployment.ID,
		Status:       "closed",
		Message:      "Locks released",
	})
}

// RegisterPlan registra que um terraform plan foi executado
// POST /api/v1/plans
//
// IDEMPOTENTE: Se deployment já existe para este PR, atualiza em vez de criar novo
func (c *PlansController) RegisterPlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.FromContext(ctx)

	// 1. Parse request
	var req api.RegisterPlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Err(err).Msg("Failed to parse register plan request body.")
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	// 2. Validações básicas
	if !req.Validate() {
		logger.Warn().
			Str("repo", req.Repo).
			Int("pr_number", req.PRNumber).
			Str("head_sha", req.HeadSHA).
			Str("SourceBranch", req.SourceBranch).
			Str("TargetBranch", req.TargetBranch).
			Msg("Missing required fields in register plan request.")
		http.Error(w, "missing required fields", http.StatusBadRequest)
		return
	}
	repo := middleware.RepoFrom(ctx)

	logger.Info().
		Str("managed repo", req.Repo).
		Int("pr", req.PRNumber).
		Str("sha", req.HeadSHA).
		Int("stacks", len(req.Stacks)).
		Msg("Registering plan from CLI")

	// 3. IDEMPOTÊNCIA: Buscar deployment existente ou criar novo
	var deployment *model.Deployment
	existing, err := c.Persistence.GetDeploymentByPR(ctx, repo.ID, req.PRNumber)

	if err == nil {
		// Deployment EXISTE - ATUALIZAR
		if existing.SourceBranch != req.SourceBranch {
			logger.Err(ErrSourceBranchChanged).Msgf("Invalid SourceBranch Changed!!!! from %s to %s", existing.SourceBranch, req.SourceBranch)
		}
		logger.Info().
			Str("deployment_id", existing.ID).
			Int("current_version", existing.PlanVersion).
			Msg("Updating existing deployment with new plan")

		deployment = existing
		deployment.HeadSHA = req.HeadSHA
		deployment.TargetBranch = req.TargetBranch
		deployment.SourceBranchSHA = req.SourceBranchSHA
		deployment.PlanOutput = req.PlanOutput
		deployment.Stacks = req.Stacks
		deployment.PlanSucceeded = true
		deployment.PlanAt = time.Now()
		deployment.PlanVersion = existing.PlanVersion + 1 // Incrementa versão
		deployment.Status = model.DeploymentPlanned
		// User pode mudar se outro dev fizer plan
		if req.User != "" {
			deployment.User = req.User
		}

	} else {
		// Deployment NÃO EXISTE - CRIAR NOVO
		logger.Info().Msg("Creating new deployment for PR")

		deployment = &model.Deployment{
			ID:              uuid.New().String(),
			PRNumber:        req.PRNumber,
			RepoID:          repo.ID,
			User:            req.User,
			CreatedAt:       time.Now(),
			Status:          model.DeploymentPlanned,
			HeadSHA:         req.HeadSHA,
			SourceBranchSHA: req.SourceBranchSHA,
			TargetBranch:    req.TargetBranch,
			SourceBranch:    req.SourceBranch,
			PlanOutput:      req.PlanOutput,
			Stacks:          req.Stacks,
			PlanSucceeded:   true,
			PlanAt:          time.Now(),
			PlanVersion:     1, // Primeira versão
		}
	}

	// 5. Salvar no banco (cria se novo, atualiza se existe)
	if err := c.Persistence.SaveDeployment(ctx, deployment); err != nil {
		logger.Error().Err(err).Msg("Failed to save deployment")
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	logger.Info().
		Str("deployment_id", deployment.ID).
		Int("plan_version", deployment.PlanVersion).
		Bool("is_update", existing != nil).
		Msg("Plan registered successfully")

	// 6. NOVO: Postar comentário no PR com o resultado do plan
	if err := c.postPlanComment(ctx, *deployment); err != nil {
		// Log error mas não falha a request - comentário é best-effort
		logger.Error().
			Err(err).
			Str("deployment_id", deployment.ID).
			Int("pr_number", deployment.PRNumber).
			Msg("Failed to post plan comment to PR")
	} else {
		logger.Info().
			Str("deployment_id", deployment.ID).
			Int("pr_number", deployment.PRNumber).
			Msg("Successfully posted plan comment to PR")
	}

	// 7. Responder com sucesso
	resp := api.RegisterPlanResponse{
		DeploymentID: deployment.ID,
		Status:       "planned",
		PlanVersion:  deployment.PlanVersion,
		Message:      "Plan registered successfully",
	}

	logger.Info().
		Str("deployment_id", deployment.ID).
		Int("plan_version", deployment.PlanVersion).
		Bool("is_update", existing != nil).
		Msg("Plan registered successfully")

	httputil.RespondJSON(w, http.StatusCreated, resp)
}

// postPlanComment posta o resultado do plan como comentário no PR
func (c *PlansController) postPlanComment(ctx context.Context, deployment model.Deployment) error {
	logger := log.FromContext(ctx)

	// Formatar comentário em markdown
	comment := FormatPlanComment(deployment)

	// Parse repo format: "org/repo" ou apenas "repo" (Azure DevOps)
	// Para Azure DevOps, o "owner" não é usado, então podemos passar vazio
	owner := ""
	repo := deployment.RepoID

	logger.Debug().
		Str("repo", repo).
		Int("pr_number", deployment.PRNumber).
		Int("comment_length", len(comment)).
		Msg("Posting plan comment to PR")

	// Chamar SCM para criar comentário
	if err := c.SCM.Comment(ctx, owner, repo, deployment.PRNumber, comment); err != nil {
		logger.Err(err).Msgf("Failed to post comment to PR: %v", comment)
		return err
	}

	return nil
}

// TODO: Adicionar método GetPlan para consultar plans existentes
// GET /api/v1/plans/:deployment_id
// func (c *PlansController) GetPlan(w http.ResponseWriter, r *http.Request) {
//     // Retorna informações sobre um plan específico
// }

// TODO: Adicionar método ListPlans para listar plans de um PR
// GET /api/v1/plans?repo=org/repo&pr=123
// func (c *PlansController) ListPlans(w http.ResponseWriter, r *http.Request) {
//     // Lista todos os plans de um PR específico
// }

func (c *PlansController) Approve(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.FromContext(ctx)

	// 1. Parse request
	var req api.ApproveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	// 2. Validações básicas
	if req.Repo == "" || req.PRNumber == 0 || req.HeadSHA == "" {
		http.Error(w, "missing required fields", http.StatusBadRequest)
		return
	}
	repo := middleware.RepoFrom(ctx)

	// 3. Buscar deployment existente
	var deployment model.Deployment
	existing, err := c.Persistence.GetDeploymentByPR(ctx, repo.ID, req.PRNumber)
	if err != nil {
		logger.Info().Msg("Plan Deployment not found")
		http.Error(w, "deployment not found", http.StatusNotFound)
		return
	}
	logger.Info().
		Str("managed repo", req.Repo).
		Int("pr", req.PRNumber).
		Str("sha", req.HeadSHA).
		Int("stacks", len(existing.Stacks)).
		Msg("Approve Plan")
	// 5. SUCESSO: Gera Tokens
	jitToken := uuid.New().String()
	jobID := uuid.New().String()

	if err := c.Persistence.SaveJob(ctx, model.Job{
		ID:           jobID,
		DeploymentID: existing.ID,
		RepoID:       existing.RepoID,
		Status:       model.StatusQueued,
		UpdatedAt:    time.Now(),
		JITToken:     jitToken,
	}); err != nil {
		logger.Err(err).Msg("Failed to save job.")
		http.Error(w, "failed to save job", http.StatusInternalServerError)
		return
	}

	// 7. Responder com sucesso
	resp := api.ApproveResponse{
		JobID:    jobID,
		JobToken: jitToken,
	}
	existing.Status = model.DeploymentApproved
	err = c.Persistence.SaveDeployment(ctx, existing)
	if err != nil {
		logger.Err(err).Msg("Failed to save job.")
		http.Error(w, "failed to save job", http.StatusInternalServerError)
	}

	logger.Info().
		Str("deployment_id", deployment.ID).
		Int("plan_version", deployment.PlanVersion).
		Bool("is_update", err == nil).
		Msg("Plan registered successfully")

	httputil.RespondJSON(w, http.StatusCreated, resp)
}
