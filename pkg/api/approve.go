package api

import (
	"github.com/ServerPlace/iac-controller/pkg/hmac"
)

type ApproveRequest struct {
	hmac.Signature        // ← EMBEDDING = sem duplicação de Timestamp/SignatureRequest
	Repo           string `json:"repo"`
	PRNumber       int    `json:"pr_number,omitempty"`
	HeadSHA        string `json:"head_sha,omitempty"`
}

type ApproveResponse struct {
	JobID    string `json:"job_id,omitempty"`
	JobToken string `json:"job_token,omitempty"`
}
