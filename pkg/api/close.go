package api

import "github.com/ServerPlace/iac-controller/pkg/hmac"

type ClosePlanRequest struct {
	hmac.Signature
	Repo     string `json:"repo"`
	PRNumber int    `json:"pr_number"`
	HeadSHA  string `json:"head_sha"`
}

type ClosePlanResponse struct {
	DeploymentID string `json:"deployment_id"`
	Status       string `json:"status"`
	Message      string `json:"message"`
}
