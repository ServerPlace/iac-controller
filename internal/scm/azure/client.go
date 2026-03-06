package azure

import (
	"context"
	"errors"
	"fmt"
	"github.com/ServerPlace/iac-controller/pkg/log"
	"path/filepath"
	"strings"
	"time"

	"github.com/ServerPlace/iac-controller/internal/config"
	"github.com/ServerPlace/iac-controller/internal/core/model"
	"github.com/ServerPlace/iac-controller/internal/scm"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/git"
)

type AzureClient struct {
	Connection      *azuredevops.Connection
	GitClient       git.Client
	Project         string
	WebhookUsername string
	WebhookPassword string
}

func NewClient(ctx context.Context, cfg config.Config) (*AzureClient, error) {
	connection := azuredevops.NewPatConnection(cfg.ADOOrgURL, cfg.ADOPAT)

	gitClient, err := git.NewClient(ctx, connection)
	if err != nil {
		return nil, fmt.Errorf("failed to create azure git client: %w", err)
	}

	return &AzureClient{
		Connection:      connection,
		GitClient:       gitClient,
		Project:         cfg.ADOProject,
		WebhookUsername: cfg.ADOWebhookUsername,
		WebhookPassword: cfg.ADOWebhookPassword,
	}, nil
}

// FetchRepositoryMetadata busca metadados completos do repositório do Azure DevOps
// Aceita URI (https://dev.azure.com/org/proj/_git/repo) ou GUID do repositório
func (c *AzureClient) FetchRepositoryMetadata(ctx context.Context, identifier string) (*model.RepositoryMetadata, error) {
	var org, project, repoNameOrGUID string

	// Tenta detectar se identifier é URI ou GUID
	if strings.HasPrefix(identifier, "http") {
		// É URI - parseia
		components, err := ParseURI(identifier)
		if err != nil {
			return nil, fmt.Errorf("failed to parse Azure URI: %w", err)
		}
		org = components.Organization
		project = components.Project
		repoNameOrGUID = components.RepoName
	} else {
		// Assume que é GUID ou nome do repo
		// Para Azure, precisamos saber org/project para fazer chamadas à API
		// Como não temos, vamos usar as configurações do client (já instanciado com org/project)
		// Isso significa que o client já está configurado para uma org/project específica

		// IMPORTANTE: Para registro inicial, SEMPRE use URI
		// GUID só funciona se o client já estiver configurado com org/project corretos
		return nil, fmt.Errorf("Azure client requires URI for repository lookup (format: https://dev.azure.com/org/project/_git/repo)")
	}

	// Busca o repositório via API
	// GetRepository aceita tanto nome quanto GUID
	repo, err := c.GitClient.GetRepository(ctx, git.GetRepositoryArgs{
		RepositoryId: &repoNameOrGUID,
		Project:      &project,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get repository from Azure API: %w", err)
	}

	// Valida resposta
	if repo.Id == nil {
		return nil, fmt.Errorf("repository ID is nil in Azure API response")
	}
	if repo.Name == nil {
		return nil, fmt.Errorf("repository name is nil in Azure API response")
	}

	repoGUID := repo.Id.String()
	repoName := *repo.Name

	// Extrai ProjectID se disponível
	projectID := ""
	if repo.Project != nil && repo.Project.Id != nil {
		projectID = repo.Project.Id.String()
	}

	// Constrói URI canônica
	repoURI := BuildURI(org, project, repoName)

	// Monta RepositoryMetadata
	metadata := &model.RepositoryMetadata{
		ID:          repoGUID,
		Name:        repoName,
		RepoURI:     repoURI,
		SCMProvider: model.SCMProviderAzure,
		Azure: &model.AzureMetadata{
			Organization: org,
			Project:      project,
			ProjectID:    projectID,
			RepoGUID:     repoGUID,
		},
	}

	return metadata, nil
}
func (c *AzureClient) BranchStatus(ctx context.Context, owner, repo string, sourceBranch, targetBranch string) (*scm.GitBranchStats, error) {
	logger := log.FromContext(ctx)
	// GetBranch => branch stats (ahead/behind) :contentReference[oaicite:2]{index=2}
	stats, err := c.GitClient.GetBranch(ctx, git.GetBranchArgs{
		Project:      &c.Project,
		RepositoryId: &repo,
		Name:         &sourceBranch,
		BaseVersionDescriptor: &git.GitVersionDescriptor{
			Version:     &targetBranch,
			VersionType: &git.GitVersionTypeValues.Branch, // base = target branch
		},
	})
	if err != nil {
		logger.Err(err).Msg("Failed to fetch branch")
		return nil, err
	}
	return &scm.GitBranchStats{
		Name:        stringNil(stats.Name),
		AheadCount:  intNil(stats.AheadCount),
		BehindCount: intNil(stats.BehindCount),
	}, nil
}
func intNil(i *int) int {
	if i == nil {
		return 0
	}
	return *i
}
func stringNil(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (c *AzureClient) GetPullRequest(ctx context.Context, owner, repo string, number int) (*scm.PullRequest, error) {
	prID := number

	adoPR, err := c.GitClient.GetPullRequest(ctx, git.GetPullRequestArgs{
		RepositoryId:  &repo,
		PullRequestId: &prID,
		Project:       &c.Project,
	})
	if err != nil {
		return nil, err
	}

	state := "open"
	if adoPR.Status != nil {
		switch *adoPR.Status {
		case git.PullRequestStatusValues.Completed, git.PullRequestStatusValues.Abandoned:
			state = "closed"
		}
	}

	isMerged := adoPR.Status != nil && *adoPR.Status == git.PullRequestStatusValues.Completed

	headSHA := ""
	if adoPR.LastMergeSourceCommit != nil && adoPR.LastMergeSourceCommit.CommitId != nil {
		headSHA = *adoPR.LastMergeSourceCommit.CommitId
	}

	baseRef := ""
	if adoPR.TargetRefName != nil {
		baseRef = strings.TrimPrefix(*adoPR.TargetRefName, "refs/heads/")
	}

	author := "unknown"
	if adoPR.CreatedBy != nil && adoPR.CreatedBy.DisplayName != nil {
		author = *adoPR.CreatedBy.DisplayName
	}

	return &scm.PullRequest{
		Number:     number,
		HeadSHA:    headSHA,
		BaseRef:    baseRef,
		Author:     author,
		State:      state,
		IsMerged:   isMerged,
		IsApproved: true,
		UpdatedAt:  time.Now(),
	}, nil
}

//func (c *AzureClient) Comment(ctx context.Context, owner, repo string, number int, body string) error {
//	prID := number
//	thread := git.GitPullRequestCommentThread{
//		Comments: &[]git.Comment{
//			{
//				Content:     &body,
//				CommentType: &git.CommentTypeValues.Text,
//			},
//		},
//		Status: &git.CommentThreadStatusValues.Active,
//	}
//
//	_, err := c.GitClient.CreateThread(ctx, git.CreateThreadArgs{
//		RepositoryId:  &repo,
//		PullRequestId: &prID,
//		Project:       &c.Project,
//		CommentThread: &thread,
//	})
//	return err
//}

// GetChangedStacks Refatorado com Type Switch
func (c *AzureClient) GetChangedStacks(ctx context.Context, owner, repo string, number int) ([]string, error) {
	prID := number

	iterations, err := c.GitClient.GetPullRequestIterations(ctx, git.GetPullRequestIterationsArgs{
		RepositoryId:  &repo,
		PullRequestId: &prID,
		Project:       &c.Project,
	})
	if err != nil {
		return nil, err
	}
	if iterations == nil || len(*iterations) == 0 {
		return []string{}, nil
	}
	lastIteration := (*iterations)[len(*iterations)-1]

	changes, err := c.GitClient.GetPullRequestIterationChanges(ctx, git.GetPullRequestIterationChangesArgs{
		RepositoryId:  &repo,
		PullRequestId: &prID,
		Project:       &c.Project,
		IterationId:   lastIteration.Id,
	})
	if err != nil {
		return nil, err
	}

	// Helper para extrair path do Item (que é interface{})
	getPathFromItem := func(item interface{}) *string {
		if item == nil {
			return nil
		}
		switch v := item.(type) {
		case *git.GitItem:
			return v.Path
		case git.GitItem:
			return v.Path
		default:
			return nil
		}
	}

	stackMap := make(map[string]bool)
	if changes.ChangeEntries != nil {
		for _, entry := range *changes.ChangeEntries {
			var path *string

			// 1. Tenta pegar do Item (Add/Edit)
			path = getPathFromItem(entry.Item)

			// 2. CORREÇÃO: Se falhar, pega do OriginalPath (Delete/Rename)
			// OriginalPath é *string direto, não é um Item interface.
			if path == nil && entry.OriginalPath != nil {
				path = entry.OriginalPath
			}

			if path != nil {
				cleanPath := strings.TrimPrefix(*path, "/")
				dir := filepath.Dir(cleanPath)
				if dir == "." || dir == "/" || dir == "" {
					continue
				}
				stackMap[dir] = true
			}
		}
	}

	result := make([]string, 0, len(stackMap))
	for s := range stackMap {
		result = append(result, s)
	}
	return result, nil
}

func (c *AzureClient) MergePR(ctx context.Context, owner, repo string, number int, headSHA string) error {
	completed := git.PullRequestStatusValues.Completed
	deleteSourceBranch := true
	logger := log.FromContext(ctx)
	logger.Debug().Msgf("Merge pull request #%d from %s to %s.", number, owner, repo)
	_, err := c.GitClient.UpdatePullRequest(ctx, git.UpdatePullRequestArgs{
		RepositoryId:  &repo,
		PullRequestId: &number,
		Project:       &c.Project,
		GitPullRequestToUpdate: &git.GitPullRequest{
			Status:                &completed,
			LastMergeSourceCommit: &git.GitCommitRef{CommitId: &headSHA},
			CompletionOptions: &git.GitPullRequestCompletionOptions{
				DeleteSourceBranch: &deleteSourceBranch,
			},
		},
	})
	return err
}

func (c *AzureClient) SetStatus(ctx context.Context, owner, repo, sha string, state string, description string, targetURL string) error {
	var gitState git.GitStatusState
	switch state {
	case "success":
		gitState = git.GitStatusStateValues.Succeeded
	case "failure":
		gitState = git.GitStatusStateValues.Failed
	case "pending":
		gitState = git.GitStatusStateValues.Pending
	case "error":
		gitState = git.GitStatusStateValues.Error
	default:
		gitState = git.GitStatusStateValues.Pending
	}

	ctxName := "IaC-Controller"
	genre := "infrastructure"

	status := &git.GitStatus{
		Description: &description,
		State:       &gitState,
		TargetUrl:   &targetURL,
		Context: &git.GitStatusContext{
			Name:  &ctxName,
			Genre: &genre,
		},
	}

	_, err := c.GitClient.CreateCommitStatus(ctx, git.CreateCommitStatusArgs{
		Project:                 &c.Project,
		RepositoryId:            &repo,
		CommitId:                &sha,
		GitCommitStatusToCreate: status,
	})
	return err
}

// Comment creates a new PR thread. Always posts a new comment, no deduplication.
func (c *AzureClient) Comment(ctx context.Context, owner, repo string, number int, body string) error {
	_, err := c.createThread(ctx, repo, number, body)
	return err
}

// CommentUpdate upserts a PR comment identified by key.
// If a previous comment with the same key exists it is updated; otherwise a new one is created.
func (c *AzureClient) CommentUpdate(ctx context.Context, owner, repo string, number int, key, body string) error {
	prID := number

	mk := botMarker(key)
	bodyWithMarker := injectMarker(body, mk)

	// 1) tenta reencontrar o comment do bot (sem depender do DB)
	ref, err := c.findBotCommentByMarker(ctx, repo, prID, mk)
	if err != nil {
		return err
	}

	// 2) não achou -> cria novo
	if ref == nil {
		_, err := c.createThread(ctx, repo, prID, bodyWithMarker)
		return err
	}

	// 3) achou -> tenta atualizar
	if err := c.updateComment(ctx, repo, prID, ref.ThreadID, ref.CommentID, bodyWithMarker); err == nil {
		return nil
	} else {
		// 4) se sumiu (apagaram), update costuma dar 404 -> recria
		if isNotFound(err) {
			_, err2 := c.createThread(ctx, repo, prID, bodyWithMarker)
			return err2
		}
		return err
	}
}

// ---- internos ----

type prCommentRef struct {
	ThreadID  int
	CommentID int
	Content   string
}

func botMarker(key string) string {
	return fmt.Sprintf("<!-- bot:iac-controller key=%s -->", key)
}

func injectMarker(body, marker string) string {
	if strings.Contains(body, marker) {
		return body
	}
	return body + "\n\n" + marker
}

func (c *AzureClient) createThread(ctx context.Context, repo string, prID int, body string) (*prCommentRef, error) {
	thread := git.GitPullRequestCommentThread{
		Comments: &[]git.Comment{
			{
				Content:     &body,
				CommentType: &git.CommentTypeValues.Text,
			},
		},
		Status: &git.CommentThreadStatusValues.Active,
	}

	created, err := c.GitClient.CreateThread(ctx, git.CreateThreadArgs{
		RepositoryId:  &repo,
		PullRequestId: &prID,
		Project:       &c.Project,
		CommentThread: &thread,
	})
	if err != nil {
		return nil, err
	}

	// Extrai IDs do retorno (útil se depois você quiser persistir)
	if created == nil || created.Id == nil {
		return nil, fmt.Errorf("CreateThread returned nil thread id")
	}
	if created.Comments == nil || len(*created.Comments) == 0 || (*created.Comments)[0].Id == nil {
		return nil, fmt.Errorf("CreateThread returned nil comment id")
	}

	return &prCommentRef{
		ThreadID:  *created.Id,
		CommentID: *(*created.Comments)[0].Id,
	}, nil
}

func (c *AzureClient) findBotCommentByMarker(ctx context.Context, repo string, prID int, marker string) (*prCommentRef, error) {
	threads, err := c.GitClient.GetThreads(ctx, git.GetThreadsArgs{
		RepositoryId:  &repo,
		PullRequestId: &prID,
		Project:       &c.Project,
	})
	if err != nil {
		return nil, err
	}
	if threads == nil {
		return nil, nil
	}

	for _, th := range *threads {
		if th.Id == nil || th.Comments == nil {
			continue
		}
		tid := *th.Id

		for _, cm := range *th.Comments {
			if cm.Id == nil || cm.Content == nil {
				continue
			}
			if strings.Contains(*cm.Content, marker) {
				return &prCommentRef{ThreadID: tid, CommentID: *cm.Id, Content: *cm.Content}, nil
			}
		}
	}
	return nil, nil
}

func (c *AzureClient) updateComment(ctx context.Context, repo string, prID, threadID, commentID int, newBody string) error {
	_, err := c.GitClient.UpdateComment(ctx, git.UpdateCommentArgs{
		RepositoryId:  &repo,
		PullRequestId: &prID,
		ThreadId:      &threadID,
		CommentId:     &commentID,
		Project:       &c.Project,
		Comment: &git.Comment{
			Content: &newBody,
		},
	})
	return err
}

func isNotFound(err error) bool {
	var we azuredevops.WrappedError
	if errors.As(err, &we) && we.StatusCode != nil && *we.StatusCode == 404 {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "404") || strings.Contains(msg, "not found")
}
