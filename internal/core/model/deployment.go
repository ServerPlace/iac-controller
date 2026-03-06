package model

import "time"

type DeploymentStatus string

const (
	DeploymentPlanned  DeploymentStatus = "planned"
	DeploymentApproved DeploymentStatus = "approved"
	DeploymentRejected DeploymentStatus = "rejected"
	DeploymentPending  DeploymentStatus = "pending"
	DeploymentFailed   DeploymentStatus = "failed"
	DeploymentRunning  DeploymentStatus = "running"
	DeploymentApplied  DeploymentStatus = "applied"
	DeploymentMerged   DeploymentStatus = "merged"
)

// Deployment representa um Pull Request e seu estado no sistema
// Este model será renomeado para PullRequest no futuro (cleanup)
type Deployment struct {
	ID        string           `firestore:"id"`
	PRNumber  int              `firestore:"pr_number"`
	RepoID    string           `firestore:"repo_id"`
	User      string           `firestore:"user"`
	CreatedAt time.Time        `firestore:"created_at"`
	Status    DeploymentStatus `firestore:"status"`

	// ===== NOVOS CAMPOS PARA PLAN =====
	// SourceBranch of plan
	SourceBranch string `firestore:"source_branch"`
	// TargetBranch of plan
	TargetBranch string `firestore:"target_branch"`
	// HeadSHA é o commit SHA do PR no momento do plan (pode ser merge commit em Azure PR builds)
	HeadSHA string `firestore:"head_sha,omitempty"`

	// SourceBranchSHA é o SHA do tip da branch de origem (SYSTEM_PULLREQUEST_SOURCECOMMITID no Azure)
	// Usado para MergePR — LastMergeSourceCommit no Azure DevOps
	SourceBranchSHA string `firestore:"source_branch_sha,omitempty"`

	// PlanOutput contém o output completo do terraform plan
	// Usado para exibir no PR comment e para validações
	PlanOutput string `firestore:"plan_output,omitempty"`

	// Stacks é a lista de stacks afetados neste PR
	// Ex: ["prod/vpc", "prod/compute"]
	Stacks []string `firestore:"stacks,omitempty"`

	// PlanSucceeded indica se o último plan foi bem-sucedido
	PlanSucceeded bool `firestore:"plan_succeeded"`

	// PlanAt é o timestamp do último plan
	PlanAt time.Time `firestore:"plan_at,omitempty"`

	// PlanVersion incrementa a cada novo plan (para invalidar plans antigos)
	PlanVersion int `firestore:"plan_version"`
}
