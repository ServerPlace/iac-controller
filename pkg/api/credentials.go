package api

import (
	"github.com/ServerPlace/iac-controller/pkg/hmac"
	"time"
)

const (
	ModePlan  = "plan"
	ModeApply = "apply"
)

// ============================================
// CREDENTIALS
// ============================================

// CredentialsRequest usa composition com SecureRequest
// Campos Timestamp e SignatureRequest vêm de SecureRequest (promoted fields)
type CredentialsRequest struct {
	hmac.Signature // ← EMBEDDING = sem duplicação de Timestamp/SignatureRequest

	Mode            string   `json:"mode"`
	Repo            string   `json:"repo"`
	PRNumber        string   `json:"pr_number,omitempty"`
	HeadSHA         string   `json:"head_sha,omitempty"`
	SourceBranchSHA string   `json:"source_branch_sha,omitempty"`
	SourceBranch    string   `json:"source_branch,omitempty"`
	TargetBranch    string   `json:"target_branch,omitempty"`
	Stacks          []string `json:"stacks,omitempty"`
	JobID           string   `json:"job_id,omitempty"`
	JobToken        string   `json:"job_token,omitempty"`
}

type CredentialsResponse struct {
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
	Project     string    `json:"project"`
}
