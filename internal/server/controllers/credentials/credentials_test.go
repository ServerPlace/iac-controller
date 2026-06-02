package credentials

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ServerPlace/iac-controller/internal/compliance"
	"github.com/ServerPlace/iac-controller/internal/config"
	"github.com/ServerPlace/iac-controller/internal/core/model"
	"github.com/ServerPlace/iac-controller/internal/core/ports"
	"github.com/ServerPlace/iac-controller/internal/scm"
	"github.com/ServerPlace/iac-controller/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// defaultEngine builds an engine with only {sha_stale, branch_up_to_date} — the safe defaults.
func defaultEngine(t *testing.T) *compliance.ApplyEngine {
	t.Helper()
	e, err := compliance.BuildApplyEngine(nil)
	require.NoError(t, err)
	return e
}

func TestHandleApply_StaleCommit_Returns403AndCommentsPR(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := ports.NewMockRepository(ctrl)
	mockSCM := scm.NewMockClient(ctrl)

	controller := NewCredentialsController(config.Config{}, mockRepo, mockSCM, nil, defaultEngine(t))

	const (
		repoID = "repo-guid-123"
		prNum  = 99
		cliSHA = "aaaaaa1"
		prSHA  = "bbbbbbb" // different → stale
	)

	mockRepo.EXPECT().
		GetRepositoryByID(gomock.Any(), repoID).
		Return(&model.RepositoryMetadata{ID: repoID, Name: "my-repo"}, nil)

	mockSCM.EXPECT().
		GetPullRequest(gomock.Any(), repoID, repoID, prNum).
		Return(&scm.PullRequest{Number: prNum, HeadSHA: prSHA, IsApproved: true}, nil)

	// branch info absent → no BranchStatus call
	// deployment absent → best-effort
	mockRepo.EXPECT().
		GetDeploymentByPR(gomock.Any(), repoID, prNum).
		Return(nil, errors.New("not found"))

	mockSCM.EXPECT().
		GetPRPolicyStatus(gomock.Any(), repoID, prNum).
		Return(nil, nil)

	// gate engine posts a single "apply-gate" comment
	mockSCM.EXPECT().
		CommentUpdate(gomock.Any(), repoID, repoID, prNum, "apply-gate", gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, _ int, _, body string) error {
			assert.Contains(t, body, cliSHA)
			assert.Contains(t, body, prSHA)
			return nil
		})

	req := api.CredentialsRequest{
		Mode:            api.ModeApply,
		Repo:            repoID,
		PRNumber:        "99",
		HeadSHA:         prSHA,
		SourceBranchSHA: cliSHA,
	}
	body, _ := json.Marshal(req)

	r := httptest.NewRequest(http.MethodPost, "/v1/credentials", bytes.NewReader(body))
	w := httptest.NewRecorder()

	controller.handleApply(w, r, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "apply gate failed")
}

func TestHandleApply_BranchBehind_Returns403(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := ports.NewMockRepository(ctrl)
	mockSCM := scm.NewMockClient(ctrl)

	controller := NewCredentialsController(config.Config{}, mockRepo, mockSCM, nil, defaultEngine(t))

	const (
		repoID = "repo-abc"
		prNum  = 42
		sha    = "abc123"
	)

	mockRepo.EXPECT().
		GetRepositoryByID(gomock.Any(), repoID).
		Return(&model.RepositoryMetadata{ID: repoID}, nil)

	mockSCM.EXPECT().
		GetPullRequest(gomock.Any(), repoID, repoID, prNum).
		Return(&scm.PullRequest{Number: prNum, HeadSHA: sha, IsApproved: true}, nil)

	mockSCM.EXPECT().
		BranchStatus(gomock.Any(), "", repoID, "feature/x", "main").
		Return(&scm.GitBranchStats{BehindCount: 2}, nil)

	mockRepo.EXPECT().
		GetDeploymentByPR(gomock.Any(), repoID, prNum).
		Return(nil, errors.New("not found"))

	mockSCM.EXPECT().
		GetPRPolicyStatus(gomock.Any(), repoID, prNum).
		Return(nil, nil)

	mockSCM.EXPECT().
		CommentUpdate(gomock.Any(), repoID, repoID, prNum, "apply-gate", gomock.Any())

	req := api.CredentialsRequest{
		Mode:            api.ModeApply,
		Repo:            repoID,
		PRNumber:        "42",
		HeadSHA:         sha,
		SourceBranchSHA: sha,
		SourceBranch:    "feature/x",
		TargetBranch:    "main",
	}
	body, _ := json.Marshal(req)

	r := httptest.NewRequest(http.MethodPost, "/v1/credentials", bytes.NewReader(body))
	w := httptest.NewRecorder()

	controller.handleApply(w, r, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestHandleApply_BranchStatusError_DeniesAccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := ports.NewMockRepository(ctrl)
	mockSCM := scm.NewMockClient(ctrl)

	controller := NewCredentialsController(config.Config{}, mockRepo, mockSCM, nil, defaultEngine(t))

	const (
		repoID = "repo-err"
		prNum  = 1
		sha    = "c0ffee"
	)

	mockRepo.EXPECT().
		GetRepositoryByID(gomock.Any(), repoID).
		Return(&model.RepositoryMetadata{ID: repoID}, nil)

	mockSCM.EXPECT().
		GetPullRequest(gomock.Any(), repoID, repoID, prNum).
		Return(&scm.PullRequest{Number: prNum, HeadSHA: sha}, nil)

	mockSCM.EXPECT().
		BranchStatus(gomock.Any(), "", repoID, "feat", "main").
		Return(nil, errors.New("scm unavailable"))

	// fail-closed before reaching the engine — no GetDeploymentByPR, GetPRPolicyStatus, or CommentUpdate

	req := api.CredentialsRequest{
		Mode:            api.ModeApply,
		Repo:            repoID,
		PRNumber:        "1",
		HeadSHA:         sha,
		SourceBranchSHA: sha,
		SourceBranch:    "feat",
		TargetBranch:    "main",
	}
	body, _ := json.Marshal(req)

	r := httptest.NewRequest(http.MethodPost, "/v1/credentials", bytes.NewReader(body))
	w := httptest.NewRecorder()

	controller.handleApply(w, r, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}
