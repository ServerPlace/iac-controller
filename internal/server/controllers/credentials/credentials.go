package credentials

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/ServerPlace/iac-controller/internal/compliance"
	"github.com/ServerPlace/iac-controller/internal/config"
	"github.com/ServerPlace/iac-controller/internal/core/model"
	"github.com/ServerPlace/iac-controller/internal/core/ports"
	"github.com/ServerPlace/iac-controller/internal/credentials"
	"github.com/ServerPlace/iac-controller/internal/iam"
	"github.com/ServerPlace/iac-controller/internal/scm"
	"github.com/ServerPlace/iac-controller/pkg/api"
	"github.com/ServerPlace/iac-controller/pkg/log"
)

type CredentialsController struct {
	Config      config.Config
	Persistence ports.Persistence
	SCM         scm.Client
	IAM         *iam.Service
	ApplyEngine *compliance.ApplyEngine
}

func NewCredentialsController(
	cfg config.Config,
	repoStorage ports.Persistence,
	scmClient scm.Client,
	iamSvc *iam.Service,
	applyEngine *compliance.ApplyEngine,
) *CredentialsController {
	return &CredentialsController{
		Config:      cfg,
		Persistence: repoStorage,
		SCM:         scmClient,
		IAM:         iamSvc,
		ApplyEngine: applyEngine,
	}
}

func (c *CredentialsController) Handle(w http.ResponseWriter, r *http.Request) {
	logger := log.FromContext(r.Context())

	var req api.CredentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Err(err).Msg("Failed to parse credentials request body.")
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	contextParams, err := credentials.FindContextParams(r.Context(), c.Persistence, req.Repo,
		credentials.WithNameSpace(credentials.KeyNamespace(req.Mode)),
		credentials.WithLegacyFallback(c.Config.LegacyKeyFallback),
	)
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
	if err != nil || !valid {
		ev := logger.Error().
			Str("hmac_ns", string(contextParams.NS)).
			Int("key_version", contextParams.Version).
			Bool("legacy_fallback", contextParams.LegacyFallback).
			Str("repo", req.Repo).
			Str("mode", req.Mode)
		if err != nil {
			ev.Err(err)
		}
		ev.Msg("HMAC validation failed")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	reqTime := time.Unix(req.Timestamp, 0)
	if time.Since(reqTime).Abs() > 5*time.Minute {
		logger.Error().Msg("Expired credentials request")
		http.Error(w, "request expired", http.StatusUnauthorized)
		return
	}

	if req.Mode == api.ModeApply {
		c.handleApply(w, r, req)
	} else {
		c.handlePlan(w, r)
	}
}

func (c *CredentialsController) handlePlan(w http.ResponseWriter, r *http.Request) {
	c.issueToken(w, r, api.ModePlan)
}

func (c *CredentialsController) handleApply(w http.ResponseWriter, r *http.Request, req api.CredentialsRequest) {
	ctx := r.Context()
	logger := log.FromContext(ctx)

	// 1. Parse PR number
	prNumber, err := strconv.Atoi(req.PRNumber)
	if err != nil {
		logger.Err(err).Str("pr_number", req.PRNumber).Msg("Invalid PR number format.")
		http.Error(w, "invalid PR number", http.StatusBadRequest)
		return
	}

	// 2. Resolve managed repo
	mRepoMeta, err := ports.ResolveManagedRepo(ctx, c.Persistence, req.Repo)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to resolve managed repo")
		http.Error(w, "invalid job", http.StatusForbidden)
		return
	}

	// 3. Fetch live PR state from SCM
	pr, err := c.SCM.GetPullRequest(ctx, req.Repo, mRepoMeta.ID, prNumber)
	if err != nil {
		logger.Err(err).Int("pr_number", prNumber).Msg("Failed to fetch pull request from SCM.")
		http.Error(w, "scm error", http.StatusBadGateway)
		return
	}

	// 4. Fetch branch divergence — fail closed: deny on error.
	// Skip if branch names are absent (older runner versions).
	var branchStatus *scm.GitBranchStats
	if req.SourceBranch != "" && req.TargetBranch != "" {
		bs, err := c.SCM.BranchStatus(ctx, "", mRepoMeta.ID, req.SourceBranch, req.TargetBranch)
		if err != nil {
			logger.Err(err).Msg("Failed to fetch branch status.")
			http.Error(w, "could not check branch status", http.StatusForbidden)
			return
		}
		branchStatus = bs
	}

	// 5. Fetch registered deployment (best-effort — sha_backend_match degrades to info if nil)
	deployment, _ := c.Persistence.GetDeploymentByPR(ctx, mRepoMeta.ID, prNumber)

	// 6. Fetch branch policy evaluations — fail closed: deny on error.
	// (nil, nil) means the provider does not support policy evaluation → gate skips gracefully.
	policyStatus, err := c.SCM.GetPRPolicyStatus(ctx, mRepoMeta.ID, prNumber)
	if err != nil {
		logger.Err(err).Msg("Failed to fetch policy status.")
		http.Error(w, "could not check branch policies", http.StatusForbidden)
		return
	}

	// 7. Run configurable apply gates
	ac := compliance.ApplyContext{
		PR:           pr,
		Deployment:   deployment,
		BranchStatus: branchStatus,
		PolicyStatus: policyStatus,
		PipelineSHA:  req.SourceBranchSHA,
	}
	results, ok := c.ApplyEngine.Check(ctx, ac)
	if !ok {
		report := compliance.GenerateReport(results, false)
		_ = c.SCM.CommentUpdate(ctx, mRepoMeta.ID, mRepoMeta.ID, prNumber, "apply-gate", report)
		ev := logger.Warn().Int("pr_number", prNumber)
		for _, r := range results {
			if !r.Passed && r.Severity == compliance.SeverityStop {
				ev = ev.Str("gate_"+r.RuleID, r.Message)
			}
		}
		ev.Msg("Apply gate blocked credential issuance")
		http.Error(w, "apply gate failed", http.StatusForbidden)
		return
	}
	_ = c.SCM.CommentClose(ctx, mRepoMeta.ID, mRepoMeta.ID, prNumber, "apply-gate")

	// 7b. Persist the validated apply SHA so merge_pr uses it instead of the
	// potentially stale plan-registered SHA (e.g. push with no terraform changes).
	if deployment != nil && req.SourceBranchSHA != "" && deployment.SourceBranchSHA != req.SourceBranchSHA {
		deployment.SourceBranchSHA = req.SourceBranchSHA
		_ = c.Persistence.SaveDeployment(ctx, deployment)
	}

	// 8. [Future: controller-initiated flow] Validate JIT token when the controller
	// triggered the pipeline and injected IV_JOB_ID + IV_JIT_SECRET.
	// This path is not exercised in the current user-initiated flow (JobToken == "").
	if req.JobToken != "" {
		if req.JobID == "" {
			http.Error(w, "missing job_id", http.StatusBadRequest)
			return
		}
		job, err := c.Persistence.GetJob(ctx, req.JobID)
		if err != nil {
			logger.Warn().Err(err).Str("job_id", req.JobID).Msg("Job lookup failed")
			http.Error(w, "invalid job", http.StatusForbidden)
			return
		}
		if job.JITToken == "" || job.JITToken != req.JobToken {
			logger.Warn().Str("job_id", req.JobID).Msg("JIT Token mismatch: Access Denied")
			http.Error(w, "unauthorized execution source", http.StatusForbidden)
			return
		}
		if job.Status != model.StatusQueued && job.Status != model.StatusRunning {
			logger.Warn().Str("job_id", req.JobID).Str("status", string(job.Status)).Msg("Job is not in a runnable state.")
			http.Error(w, "job expired or finished", http.StatusForbidden)
			return
		}
	}

	// 9. Acquire stack locks
	if len(req.Stacks) > 0 {
		user := req.Repo
		if deployment != nil && deployment.User != "" {
			user = deployment.User
		}
		if err := c.Persistence.AcquireBatch(ctx, mRepoMeta.ID, req.Stacks, user, prNumber); err != nil {
			logger.Warn().Err(err).Msg("Lock contention")
			http.Error(w, fmt.Sprintf("Locked: %v", err), http.StatusConflict)
			return
		}
	}

	// 10. Issue apply token
	logger.Debug().Int("pr_number", prNumber).Str("source_sha", req.SourceBranchSHA).Msg("Issuing apply token")
	c.issueToken(w, r, api.ModeApply)
}

func (c *CredentialsController) issueToken(w http.ResponseWriter, r *http.Request, mode string) {
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
