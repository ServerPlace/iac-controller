package plans

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ServerPlace/iac-controller/internal/config"
	"github.com/ServerPlace/iac-controller/internal/core/model"
	"github.com/ServerPlace/iac-controller/internal/core/ports"
	"github.com/ServerPlace/iac-controller/internal/middleware"
	"github.com/ServerPlace/iac-controller/internal/scm"
	"github.com/ServerPlace/iac-controller/pkg/api"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestRegisterPlan_NewDeployment_PostsComment(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := ports.NewMockRepository(ctrl)
	mockSCM := scm.NewMockClient(ctrl)

	cfg := config.Config{JITSecretKey: "test-secret-key"}
	controller := NewPlansController(mockRepo, cfg, mockSCM, nil)

	repoMeta := &model.RepositoryMetadata{ID: "test-repo", Name: "test-repo"}

	// Mock: deployment NÃO existe (primeiro plan)
	mockRepo.EXPECT().
		GetDeploymentByPR(gomock.Any(), "test-repo", 42).
		Return(nil, errors.New("not found"))

	// Mock: save deployment
	mockRepo.EXPECT().
		SaveDeployment(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, d *model.Deployment) error {
			assert.Equal(t, 42, d.PRNumber)
			assert.Equal(t, "test-repo", d.RepoID)
			assert.Equal(t, "abc123def456", d.HeadSHA)
			assert.Equal(t, 1, d.PlanVersion)
			assert.True(t, d.PlanSucceeded)
			return nil
		})

	// Mock: SCM comment succeeds
	mockSCM.EXPECT().
		Comment(gomock.Any(), "", "test-repo", 42, gomock.Any()).
		DoAndReturn(func(ctx context.Context, owner, repo string, number int, body string) error {
			assert.Contains(t, body, "✅ **Plan Succeeded**")
			assert.Contains(t, body, "prod/vpc")
			assert.Contains(t, body, "prod/compute")
			assert.Contains(t, body, "@john.doe")
			return nil
		})

	planOutput := `Terraform used the selected providers to generate the following execution
plan. Resource actions are indicated with the following symbols:
  + create

Terraform will perform the following actions:

  # aws_instance.web will be created
  + resource "aws_instance" "web" {
      + ami           = "ami-12345"
      + instance_type = "t2.micro"
    }

Plan: 1 to add, 0 to change, 0 to destroy.`

	reqBody := api.RegisterPlanRequest{
		Repo:         "test-repo",
		PRNumber:     42,
		HeadSHA:      "abc123def456",
		SourceBranch: "feature/my-branch",
		TargetBranch: "main",
		PlanOutput:   planOutput,
		Stacks:       []string{"prod/vpc", "prod/compute"},
		User:         "john.doe",
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/plans", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(middleware.WithRepo(req.Context(), repoMeta))

	w := httptest.NewRecorder()
	controller.RegisterPlan(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp api.RegisterPlanResponse
	json.NewDecoder(w.Body).Decode(&resp)
	assert.Equal(t, "planned", resp.Status)
	assert.Equal(t, 1, resp.PlanVersion)
	assert.NotEmpty(t, resp.DeploymentID)
}

func TestRegisterPlan_UpdateExisting_PostsNewComment(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := ports.NewMockRepository(ctrl)
	mockSCM := scm.NewMockClient(ctrl)

	cfg := config.Config{JITSecretKey: "test-secret-key"}
	controller := NewPlansController(mockRepo, cfg, mockSCM, nil)

	repoMeta := &model.RepositoryMetadata{ID: "test-repo", Name: "test-repo"}

	existingDeployment := &model.Deployment{
		ID:          "existing-deployment-123",
		PRNumber:    42,
		RepoID:      "test-repo",
		User:        "jane.doe",
		HeadSHA:     "old-sha-123",
		PlanVersion: 3, // já teve 3 plans
		Status:      model.DeploymentPlanned,
		CreatedAt:   time.Now().Add(-1 * time.Hour),
	}

	// Mock: deployment JÁ EXISTE
	mockRepo.EXPECT().
		GetDeploymentByPR(gomock.Any(), "test-repo", 42).
		Return(existingDeployment, nil)

	// Mock: save updated deployment
	mockRepo.EXPECT().
		SaveDeployment(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, d *model.Deployment) error {
			assert.Equal(t, "existing-deployment-123", d.ID)
			assert.Equal(t, 4, d.PlanVersion) // incremented
			assert.Equal(t, "new-sha-456", d.HeadSHA)
			return nil
		})

	// Mock: SCM comment succeeds
	mockSCM.EXPECT().
		Comment(gomock.Any(), "", "test-repo", 42, gomock.Any()).
		DoAndReturn(func(ctx context.Context, owner, repo string, number int, body string) error {
			assert.Contains(t, body, "Plan Version:** `#4`")
			return nil
		})

	reqBody := api.RegisterPlanRequest{
		Repo:         "test-repo",
		PRNumber:     42,
		HeadSHA:      "new-sha-456",
		SourceBranch: "feature/my-branch",
		TargetBranch: "main",
		PlanOutput:   "Plan: 2 to add, 1 to change, 0 to destroy.",
		Stacks:       []string{"prod/database"},
		User:         "john.doe",
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/plans", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(middleware.WithRepo(req.Context(), repoMeta))

	w := httptest.NewRecorder()
	controller.RegisterPlan(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp api.RegisterPlanResponse
	json.NewDecoder(w.Body).Decode(&resp)
	assert.Equal(t, "planned", resp.Status)
	assert.Equal(t, 4, resp.PlanVersion) // incremented
	assert.Equal(t, "existing-deployment-123", resp.DeploymentID)
}

func TestRegisterPlan_SCMFailure_StillSucceeds(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := ports.NewMockRepository(ctrl)
	mockSCM := scm.NewMockClient(ctrl)

	cfg := config.Config{JITSecretKey: "test-secret-key"}
	controller := NewPlansController(mockRepo, cfg, mockSCM, nil)

	repoMeta := &model.RepositoryMetadata{ID: "test-repo", Name: "test-repo"}

	// Mock: deployment NÃO existe
	mockRepo.EXPECT().
		GetDeploymentByPR(gomock.Any(), "test-repo", 42).
		Return(nil, errors.New("not found"))

	// Mock: save deployment succeeds
	mockRepo.EXPECT().
		SaveDeployment(gomock.Any(), gomock.Any()).
		Return(nil)

	// Mock: SCM comment FAILS
	mockSCM.EXPECT().
		Comment(gomock.Any(), "", "test-repo", 42, gomock.Any()).
		Return(errors.New("Azure DevOps API timeout"))

	reqBody := api.RegisterPlanRequest{
		Repo:         "test-repo",
		PRNumber:     42,
		HeadSHA:      "abc123",
		SourceBranch: "feature/my-branch",
		TargetBranch: "main",
		PlanOutput:   "Plan: 1 to add",
		Stacks:       []string{"prod/vpc"},
		User:         "john.doe",
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/plans", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(middleware.WithRepo(req.Context(), repoMeta))

	w := httptest.NewRecorder()
	controller.RegisterPlan(w, req)

	// Assert: Request STILL SUCCEEDS even though comment failed
	assert.Equal(t, http.StatusCreated, w.Code)

	var resp api.RegisterPlanResponse
	json.NewDecoder(w.Body).Decode(&resp)
	assert.Equal(t, "planned", resp.Status)
}

func TestFormatPlanComment_SuccessfulPlan(t *testing.T) {
	deployment := model.Deployment{
		ID:            "deploy-123",
		PRNumber:      42,
		RepoID:        "test-repo",
		User:          "john.doe",
		HeadSHA:       "abc123def456ghi789",
		PlanVersion:   2,
		PlanSucceeded: true,
		PlanAt:        time.Date(2026, 2, 15, 14, 30, 0, 0, time.UTC),
		PlanOutput:    "Plan: 1 to add, 0 to change, 0 to destroy.",
		Stacks:        []string{"prod/vpc", "prod/compute"},
	}

	comment := FormatPlanComment(deployment)

	assert.Contains(t, comment, "## ✅ **Plan Succeeded**")
	assert.Contains(t, comment, "**Deployment ID:** `deploy-123`")
	assert.Contains(t, comment, "**Plan Version:** `#2`")
	assert.Contains(t, comment, "**Commit SHA:** `abc123d`") // truncated
	assert.Contains(t, comment, "**Triggered by:** @john.doe")
	assert.Contains(t, comment, "**Planned at:** 2026-02-15 14:30:00 UTC")
	assert.Contains(t, comment, "### 📦 Affected Stacks")
	assert.Contains(t, comment, "- `prod/vpc`")
	assert.Contains(t, comment, "- `prod/compute`")
	assert.Contains(t, comment, "*Powered by iac-controller* 🚀")
}

func TestFormatPlanComment_FailedPlan(t *testing.T) {
	deployment := model.Deployment{
		ID:            "deploy-456",
		PRNumber:      42,
		RepoID:        "test-repo",
		PlanSucceeded: false,
		PlanVersion:   1,
		PlanAt:        time.Now(),
		PlanOutput:    "Error: provider authentication failed",
	}

	comment := FormatPlanComment(deployment)

	assert.Contains(t, comment, "## ❌ **Plan Failed**")
	assert.Contains(t, comment, "Error: provider authentication failed")
}
