package api

import "github.com/ServerPlace/iac-controller/pkg/hmac"

// RegisterPlanRequest é enviado pelo CLI após terraform plan bem-sucedido
// ============================================
// PLANS
// ============================================

// RegisterPlanRequest usa composition com SecureRequest
// Campos Timestamp e SignatureRequest vêm de SecureRequest (promoted fields)
type RegisterPlanRequest struct {
	hmac.Signature // ← EMBEDDING = sem duplicação de Timestamp/SignatureRequest

	Repo            string   `json:"repo"`
	PRNumber        int      `json:"pr_number"`
	SourceBranch    string   `json:"source_branch"`
	TargetBranch    string   `json:"target_branch"`
	HeadSHA         string   `json:"head_sha"`
	SourceBranchSHA string   `json:"source_branch_sha,omitempty"` // SHA do tip da branch de origem (não o merge commit)
	PlanOutput      string   `json:"plan_output"`
	Stacks          []string `json:"stacks"`
	User            string   `json:"user"`
}

func (r *RegisterPlanRequest) Validate() bool {
	return !(r.Repo == "" || r.PRNumber == 0 || r.HeadSHA == "" || r.SourceBranch == "" || r.TargetBranch == "")
}

type RegisterPlanResponse struct {
	DeploymentID string `json:"deployment_id"`
	Status       string `json:"status"`
	PlanVersion  int    `json:"plan_version"`
	Message      string `json:"message"`
}
