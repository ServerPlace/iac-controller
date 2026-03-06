package model

import (
	"time"
)

type OpStatus string

const (
	StatusQueued  OpStatus = "QUEUED"
	StatusRunning OpStatus = "RUNNING"
	StatusSuccess OpStatus = "SUCCESS"
	StatusFailed  OpStatus = "FAILED"
)

// Job representa uma execução de terraform (plan ou apply)
type Job struct {
	ID           string    `firestore:"id"`
	DeploymentID string    `firestore:"deployment_id"`
	RepoID       string    `firestore:"repo_id"`
	Status       OpStatus  `firestore:"status"`
	UpdatedAt    time.Time `firestore:"updated_at"`
	JITToken     string    `firestore:"jit_token"`

	// NOVO: Tipo de operação (plan ou apply)
	Operation string `firestore:"operation,omitempty"` // "plan" ou "apply"

	// NOVO: Quem disparou este job (para auditoria e locks)
	User string `firestore:"user,omitempty"`
}

// Lock representa um lock em um stack específico
// Locks NÃO expiram automaticamente - só são liberados explicitamente via:
// 1. Comando /unlock (chatops)
// 2. PR merged
// 3. PR closed
type Lock struct {
	RepoID    string    `firestore:"repo_id"`
	StackPath string    `firestore:"stack_path"`
	PRNumber  int       `firestore:"pr_number"`
	User      string    `firestore:"user"`
	CreatedAt time.Time `firestore:"created_at"` // Quando foi criado (auditoria)
}

type SCMProvider string

const (
	SCMProviderAzure  SCMProvider = "azure"
	SCMProviderGitHub SCMProvider = "github"
	SCMProviderGitLab SCMProvider = "gitlab"
)

// Valid verifica se o tipo é válido
func (t SCMProvider) Valid() bool {
	switch t {
	case SCMProviderAzure, SCMProviderGitHub, SCMProviderGitLab:
		return true
	default:
		return false
	}
}

// RepositoryMetadata contém metadados de um repositório registrado
type RepositoryMetadata struct {
	ID          string      `firestore:"id" json:"id"`                     // ID nativo do provider (GUID do Azure, número do GitHub/GitLab)
	Name        string      `firestore:"name" json:"name"`                 // Nome do repositório
	RepoURI     string      `firestore:"repo_uri" json:"repo_uri"`         // URI completa original
	SCMProvider SCMProvider `firestore:"scm_provider" json:"scm_provider"` // azure, github, gitlab
	KeyVersion  int         `firestore:"key_version" json:"key_version"`
	CreatedAt   time.Time   `firestore:"created_at" json:"created_at"`
	UpdatedAt   time.Time   `firestore:"updated_at,omitempty" json:"updated_at,omitempty"`

	// Metadados específicos por provider (apenas um será preenchido)
	Azure  *AzureMetadata  `firestore:"azure,omitempty" json:"azure,omitempty"`
	GitHub *GitHubMetadata `firestore:"github,omitempty" json:"github,omitempty"`
	GitLab *GitLabMetadata `firestore:"gitlab,omitempty" json:"gitlab,omitempty"`
}

// AzureMetadata contém campos específicos do Azure DevOps
type AzureMetadata struct {
	Organization string `firestore:"organization" json:"organization"`                 // Nome da organização
	Project      string `firestore:"project" json:"project"`                           // Nome do projeto
	ProjectID    string `firestore:"project_id,omitempty" json:"project_id,omitempty"` // GUID do projeto
	RepoGUID     string `firestore:"repo_guid" json:"repo_guid"`                       // GUID do repositório (igual ao ID principal)
}

// GitHubMetadata contém campos específicos do GitHub
type GitHubMetadata struct {
	Owner        string `firestore:"owner" json:"owner"`                 // Nome do owner (user ou org)
	RepoName     string `firestore:"repo_name" json:"repo_name"`         // Nome do repositório
	RepositoryID int64  `firestore:"repository_id" json:"repository_id"` // ID numérico do repo (igual ao ID principal convertido)
}

// GitLabMetadata contém campos específicos do GitLab
type GitLabMetadata struct {
	Namespace   string `firestore:"namespace" json:"namespace"`       // Namespace (group/subgroup)
	ProjectPath string `firestore:"project_path" json:"project_path"` // Path completo (namespace/project)
	ProjectID   int64  `firestore:"project_id" json:"project_id"`     // ID numérico do projeto (igual ao ID principal convertido)
}

// PipelineTriggerRequest é usado para disparar pipelines
type PipelineTriggerRequest struct {
	Repo      string
	Branch    string
	CommitSHA string
	Variables map[string]string
}
