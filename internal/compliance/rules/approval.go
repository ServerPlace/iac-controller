package rules

import (
	"context"
	"github.com/ServerPlace/iac-controller/internal/compliance"
	"github.com/ServerPlace/iac-controller/internal/scm"
)

func init() {
	compliance.Register("approval_check", func(_ map[string]interface{}) (compliance.Rule, error) {
		return &ApprovalCheck{}, nil
	})
}

type ApprovalCheck struct{}

func (r *ApprovalCheck) ID() string { return "approval_check" }
func (r *ApprovalCheck) Validate(_ context.Context, pr *scm.PullRequest) (compliance.Result, error) {
	if !pr.IsApproved {
		return compliance.Result{
			RuleID: r.ID(), Name: "Approval", Passed: false, Severity: compliance.SeverityStop,
			Message: "PR needs approval"}, nil
	}
	return compliance.Result{
		RuleID: r.ID(), Name: "Approval", Passed: true, Severity: compliance.SeverityStop,
		Message: "Approved"}, nil
}
