package github

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ServerPlace/iac-controller/internal/config"
	"github.com/ServerPlace/iac-controller/internal/core/model"
	"github.com/ServerPlace/iac-controller/internal/scm"
	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v57/github"
)

type GithubClient struct {
	client        *github.Client
	webhookSecret []byte
	staleLimit    time.Duration
}

func NewClient(ctx context.Context, cfg config.Config) (*GithubClient, error) {
	appID, err := strconv.ParseInt(strings.TrimSpace(cfg.GithubAppID), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid App ID: %w", err)
	}
	installID, err := strconv.ParseInt(strings.TrimSpace(cfg.GithubInstallID), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid Install ID: %w", err)
	}

	itr, err := ghinstallation.New(http.DefaultTransport, appID, installID, []byte(cfg.GithubPrivateKey))
	if err != nil {
		return nil, err
	}

	return &GithubClient{
		client:        github.NewClient(&http.Client{Transport: itr}),
		webhookSecret: []byte(cfg.GithubWebhookSecret),
		staleLimit:    24 * time.Hour,
	}, nil
}
func (g *GithubClient) BranchStatus(ctx context.Context, owner, repo string, sourceBranch, targetBranch string) (*scm.GitBranchStats, error) {
	panic("Branch Status Unimplemented in Github")
}

// FetchRepositoryMetadata busca metadados completos do repositório do GitHub
// Aceita URI (https://github.com/owner/repo) ou ID numérico
func (g *GithubClient) FetchRepositoryMetadata(ctx context.Context, identifier string) (*model.RepositoryMetadata, error) {
	var owner, repoName string
	var repoID int64

	// Tenta detectar se identifier é URI ou ID numérico
	if strings.HasPrefix(identifier, "http") || strings.HasPrefix(identifier, "git@") {
		// É URI - parseia
		components, err := ParseURI(identifier)
		if err != nil {
			return nil, fmt.Errorf("failed to parse GitHub URI: %w", err)
		}
		owner = components.Owner
		repoName = components.RepoName

		// Busca repositório via API para obter ID
		repo, _, err := g.client.Repositories.Get(ctx, owner, repoName)
		if err != nil {
			return nil, fmt.Errorf("failed to get repository from GitHub API: %w", err)
		}

		if repo.ID == nil {
			return nil, fmt.Errorf("repository ID is nil in GitHub API response")
		}
		repoID = *repo.ID

	} else {
		// Tenta parsear como ID numérico
		id, err := strconv.ParseInt(identifier, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("identifier must be a GitHub URI or numeric repository ID")
		}
		repoID = id

		// Busca repositório por ID
		repo, _, err := g.client.Repositories.GetByID(ctx, repoID)
		if err != nil {
			return nil, fmt.Errorf("failed to get repository by ID from GitHub API: %w", err)
		}

		if repo.Owner == nil || repo.Owner.Login == nil {
			return nil, fmt.Errorf("repository owner is nil in GitHub API response")
		}
		if repo.Name == nil {
			return nil, fmt.Errorf("repository name is nil in GitHub API response")
		}

		owner = *repo.Owner.Login
		repoName = *repo.Name
	}

	// Constrói URI canônica
	repoURI := BuildURI(owner, repoName)

	// Monta RepositoryMetadata
	metadata := &model.RepositoryMetadata{
		ID:          strconv.FormatInt(repoID, 10), // Converte ID numérico para string
		Name:        repoName,
		RepoURI:     repoURI,
		SCMProvider: model.SCMProviderGitHub,
		GitHub: &model.GitHubMetadata{
			Owner:        owner,
			RepoName:     repoName,
			RepositoryID: repoID,
		},
	}

	return metadata, nil
}

func (g *GithubClient) GetPullRequest(ctx context.Context, owner, repo string, number int) (*scm.PullRequest, error) {
	pr, _, err := g.client.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}

	isMerged := pr.GetMerged()
	// No GitHub, "Approved" requer checar reviews, aqui simplificamos ou chamamos outra API
	// Para este exemplo, assumimos false ou implementamos a lógica de reviews
	isApproved := false

	// Exemplo rápido de check de aprovação (opcional)
	reviews, _, _ := g.client.PullRequests.ListReviews(ctx, owner, repo, number, nil)
	for _, r := range reviews {
		if r.GetState() == "APPROVED" {
			isApproved = true
			break
		}
	}

	return &scm.PullRequest{
		Number:     pr.GetNumber(),
		HeadSHA:    pr.GetHead().GetSHA(),
		BaseRef:    pr.GetBase().GetRef(),
		Author:     pr.GetUser().GetLogin(),
		State:      pr.GetState(),
		IsMerged:   isMerged,
		IsApproved: isApproved,
		UpdatedAt:  pr.GetUpdatedAt().Time,
	}, nil
}

func (g *GithubClient) MergePR(ctx context.Context, owner, repo string, number int, headSHA string) error {
	_, _, err := g.client.PullRequests.Merge(ctx, owner, repo, number, "", &github.PullRequestOptions{
		MergeMethod: "merge",
		SHA:         headSHA,
	})
	return err
}

func ghBotMarker(key string) string {
	return fmt.Sprintf("<!-- bot:iac-controller key=%s -->", key)
}

func (g *GithubClient) findBotComment(ctx context.Context, owner, repo string, number int, marker string) (int64, string, error) {
	opts := &github.IssueListCommentsOptions{ListOptions: github.ListOptions{PerPage: 100}}
	for {
		comments, resp, err := g.client.Issues.ListComments(ctx, owner, repo, number, opts)
		if err != nil {
			return 0, "", err
		}
		for _, c := range comments {
			if c.Body != nil && strings.Contains(*c.Body, marker) {
				return c.GetID(), *c.Body, nil
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return 0, "", nil
}

// Comment creates a new PR comment. Always posts a new comment, no deduplication.
func (g *GithubClient) Comment(ctx context.Context, owner, repo string, number int, body string) error {
	comment := &github.IssueComment{Body: &body}
	_, _, err := g.client.Issues.CreateComment(ctx, owner, repo, number, comment)
	return err
}

// CommentUpdate upserts a PR comment identified by key.
// If a previous comment with the same key exists it is updated; otherwise a new one is created.
func (g *GithubClient) CommentUpdate(ctx context.Context, owner, repo string, number int, key, body string) error {
	mk := ghBotMarker(key)
	bodyWithMarker := body + "\n\n" + mk

	commentID, _, err := g.findBotComment(ctx, owner, repo, number, mk)
	if err != nil {
		return err
	}

	if commentID == 0 {
		comment := &github.IssueComment{Body: &bodyWithMarker}
		_, _, err := g.client.Issues.CreateComment(ctx, owner, repo, number, comment)
		return err
	}

	comment := &github.IssueComment{Body: &bodyWithMarker}
	_, _, err = g.client.Issues.EditComment(ctx, owner, repo, commentID, comment)
	return err
}

func (g *GithubClient) SetStatus(ctx context.Context, owner, repo, sha string, state string, description string, targetURL string) error {
	// Mapeamento de status genérico para GitHub
	ghState := "pending"
	switch state {
	case "success":
		ghState = "success"
	case "failure":
		ghState = "failure"
	case "error":
		ghState = "error"
	}

	status := &github.RepoStatus{
		State:       github.String(ghState),
		TargetURL:   github.String(targetURL),
		Description: github.String(description),
		Context:     github.String("IaC-Controller"),
	}
	_, _, err := g.client.Repositories.CreateStatus(ctx, owner, repo, sha, status)
	return err
}
