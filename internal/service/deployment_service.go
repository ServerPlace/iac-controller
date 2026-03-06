package service

import (
	"context"
	"fmt"
	"time"

	"github.com/ServerPlace/iac-controller/internal/compliance" // <--- Novo Import
	"github.com/ServerPlace/iac-controller/internal/config"
	"github.com/ServerPlace/iac-controller/internal/core/model"
	"github.com/ServerPlace/iac-controller/internal/core/ports"
	"github.com/ServerPlace/iac-controller/internal/scm"
	"github.com/google/uuid"
)

type DeploymentService struct {
	cfg        config.Config
	scm        scm.Client
	pipeline   ports.PipelineOrchestrator
	repo       ports.Persistence
	compliance *compliance.Engine // <--- Novo Campo
}

// Construtor atualizado
func NewDeploymentService(
	c config.Config,
	s scm.Client,
	p ports.PipelineOrchestrator,
	r ports.Persistence,
	comp *compliance.Engine,
) *DeploymentService {
	return &DeploymentService{
		cfg:        c,
		scm:        s,
		pipeline:   p,
		repo:       r,
		compliance: comp,
	}
}

func (s *DeploymentService) RunApply(ctx context.Context, repoID string, prNum int, user string) error {
	// 1. Dados do PR
	pr, err := s.scm.GetPullRequest(ctx, repoID, repoID, prNum)
	if err != nil {
		return fmt.Errorf("failed to get pr info: %w", err)
	}

	// 2. AUDITORIA: Registra intenção (Status: VALIDATING) antes de validar
	deployID := uuid.New().String()
	d := &model.Deployment{
		ID:        deployID,
		PRNumber:  prNum,
		RepoID:    repoID,
		User:      user,
		CreatedAt: time.Now(),
		Status:    "VALIDATING",
	}
	if err := s.repo.SaveDeployment(ctx, d); err != nil {
		return fmt.Errorf("failed to save deployment intent: %w", err)
	}

	// 3. COMPLIANCE CHECK
	results, isCompliant := s.compliance.Evaluate(ctx, pr)
	report := compliance.GenerateReport(results, isCompliant)

	// 4. DECISÃO
	if !isCompliant {
		// A. Notifica PR
		_ = s.scm.CommentUpdate(ctx, repoID, repoID, prNum, "deploy", report)

		// B. Registra Falha no Banco (Auditoria)
		_ = s.repo.SaveJob(ctx, model.Job{
			ID:           uuid.New().String(),
			DeploymentID: deployID,
			RepoID:       repoID,
			Status:       model.StatusFailed,
			UpdatedAt:    time.Now(),
			JITToken:     "BLOCKED",
		})

		return fmt.Errorf("compliance blocked execution")
	}

	// 5. SUCESSO: Gera Tokens
	jitToken := uuid.New().String()
	jobID := uuid.New().String()

	job := model.Job{
		ID:           jobID,
		DeploymentID: deployID,
		RepoID:       repoID,
		Status:       model.StatusQueued,
		UpdatedAt:    time.Now(),
		JITToken:     jitToken,
		User:         user,
	}
	if err := s.repo.SaveJob(ctx, job); err != nil {
		return fmt.Errorf("failed to save job: %w", err)
	}

	// 6. Trigger Pipeline
	req := model.PipelineTriggerRequest{
		Repo:      repoID,
		Branch:    pr.BaseRef,
		CommitSHA: pr.HeadSHA,
		Variables: map[string]string{
			"IV_JOB_ID":     jobID,
			"IV_JIT_SECRET": jitToken,
		},
	}

	url, err := s.pipeline.TriggerApply(ctx, req)
	if err != nil {
		// Rollback
		_ = s.repo.UpdateJobStatus(ctx, jobID, model.StatusFailed)
		_ = s.scm.CommentUpdate(ctx, repoID, repoID, prNum, "deploy", fmt.Sprintf("%s\n\n❌ Trigger Failed: `%s`", report, err.Error()))
		return fmt.Errorf("trigger failed: %w", err)
	}

	// 7. Feedback Final
	_ = s.repo.UpdateJobStatus(ctx, jobID, model.StatusRunning)
	return s.scm.CommentUpdate(ctx, repoID, repoID, prNum, "deploy", fmt.Sprintf("%s\n\n🚀 **Pipeline Started:** %s", report, url))
}

// ReleasePRLocks libera todos os locks de um PR
// Usado quando PR é merged, closed, ou comando /unlock
func (s *DeploymentService) ReleasePRLocks(ctx context.Context, repo string, prNumber int) error {
	return s.repo.ReleaseBatch(ctx, repo, prNumber)
}
