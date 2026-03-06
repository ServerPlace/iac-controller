package scm

import (
	"context"
	"github.com/ServerPlace/iac-controller/internal/core/model"
	"time"
)

const (
	EventComment = "COMMENT"
	EventPing    = "PING"
)

// WebhookEvent normalizado
type WebhookEvent struct {
	Type     string
	Repo     string
	PRNumber int
	Sender   string
	Body     string
}

type PullRequest struct {
	Number     int
	HeadSHA    string
	BaseRef    string
	Author     string
	State      string
	IsMerged   bool
	IsApproved bool
	UpdatedAt  time.Time
}

type GitBranchStats struct {
	Name        string
	AheadCount  int
	BehindCount int
}

// FILE: internal/scm/interface.go
//go:generate mockgen -destination=mock_client.go -package=scm github.com/ServerPlace/iac-controller/internal/scm Client

type Client interface {
	GetPullRequest(ctx context.Context, owner, repo string, number int) (*PullRequest, error)

	BranchStatus(ctx context.Context, owner, repo string, sourceBranch, targetBranch string) (*GitBranchStats, error)

	// Comment creates a new PR thread. Always posts a new comment, no deduplication.
	Comment(ctx context.Context, owner, repo string, number int, body string) error

	// CommentUpdate upserts a PR comment identified by key.
	// If a previous comment with the same key exists it is updated; otherwise a new one is created.
	CommentUpdate(ctx context.Context, owner, repo string, number int, key, body string) error

	SetStatus(ctx context.Context, owner, repo, sha string, state string, description string, targetURL string) error

	// MergePR completa (merge) o pull request especificado
	MergePR(ctx context.Context, owner, repo string, number int, headSHA string) error

	// FetchRepositoryMetadata busca metadados completos do repositório via API do provider
	// Aceita URI ou ID nativo como identificador
	// Retorna RepositoryMetadata completo com ID nativo, nome, URI e metadados específicos do provider
	FetchRepositoryMetadata(ctx context.Context, identifier string) (*model.RepositoryMetadata, error)
}
