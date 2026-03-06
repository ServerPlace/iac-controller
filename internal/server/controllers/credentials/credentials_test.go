package credentials

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ServerPlace/iac-controller/internal/config"
	"github.com/ServerPlace/iac-controller/internal/core/model"
	"github.com/ServerPlace/iac-controller/internal/core/ports"
	"github.com/ServerPlace/iac-controller/internal/scm"
	"github.com/ServerPlace/iac-controller/pkg/api"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestHandleApply_StaleCommit_Returns409AndCommentsPR(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := ports.NewMockRepository(ctrl)
	mockSCM := scm.NewMockClient(ctrl)

	// iam.Service não é alcançado no caminho de stale — pode ser nil
	controller := NewCredentialsController(config.Config{}, mockRepo, mockSCM, nil)

	const (
		repoID   = "repo-guid-123"
		jobID    = "job-uuid-456"
		jitToken = "super-secret-jit-token"
		prNum    = 99
		cliSHA   = "aaaaaa1"
		prSHA    = "bbbbbbb" // diferente → stale
	)

	// Stale check acontece antes do GetJob — GetJob não deve ser chamado
	// ResolveManagedRepo → GetRepositoryByID encontra o repo
	mockRepo.EXPECT().
		GetRepositoryByID(gomock.Any(), repoID).
		Return(&model.RepositoryMetadata{ID: repoID, Name: "my-repo"}, nil)

	// PR aprovado mas com HEAD SHA diferente (stale)
	mockSCM.EXPECT().
		GetPullRequest(gomock.Any(), repoID, repoID, prNum).
		Return(&scm.PullRequest{
			Number:     prNum,
			HeadSHA:    prSHA,
			IsApproved: true,
		}, nil)

	// Deve postar comentário de plano desatualizado no PR
	mockSCM.EXPECT().
		CommentUpdate(gomock.Any(), repoID, repoID, prNum, "stale", gomock.Any()).
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
		Stacks:          []string{"prod/vpc"},
		JobID:           jobID,
		JobToken:        jitToken,
	}
	body, _ := json.Marshal(req)

	r := httptest.NewRequest(http.MethodPost, "/v1/credentials", bytes.NewReader(body))
	w := httptest.NewRecorder()

	controller.handleApply(w, r, req)

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "stale")
}
