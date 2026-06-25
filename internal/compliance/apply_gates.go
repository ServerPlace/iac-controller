package compliance

import (
	"context"
	"fmt"
	"strings"
)

// gSHAStale blocks if the pipeline's source SHA doesn't match the PR's live HEAD.
// Prevents applying a plan generated against a different commit.
type gSHAStale struct{}

func (g *gSHAStale) ID() string { return "sha_stale" }

func (g *gSHAStale) Check(_ context.Context, ac ApplyContext) (Result, error) {
	if ac.PipelineSHA == "" || ac.PR.HeadSHA == "" {
		return Result{
			RuleID: g.ID(), Name: "SHA Stale", Passed: true,
			Severity: SeverityInfo, Message: "SHA not available — skipping",
		}, nil
	}
	if ac.PipelineSHA != ac.PR.HeadSHA {
		return Result{
			RuleID: g.ID(), Name: "SHA Stale", Passed: false,
			Severity: SeverityStop,
			Message:  fmt.Sprintf("plan SHA `%s` ≠ PR HEAD `%s` — run plan again", ac.PipelineSHA, ac.PR.HeadSHA),
		}, nil
	}
	return Result{RuleID: g.ID(), Name: "SHA Stale", Passed: true, Severity: SeverityStop, Message: "commit up to date"}, nil
}

// gBranchUpToDate blocks if the source branch is behind the target branch.
type gBranchUpToDate struct{}

func (g *gBranchUpToDate) ID() string { return "branch_up_to_date" }

func (g *gBranchUpToDate) Check(_ context.Context, ac ApplyContext) (Result, error) {
	if ac.BranchStatus == nil {
		return Result{
			RuleID: g.ID(), Name: "Branch Up To Date", Passed: true,
			Severity: SeverityInfo, Message: "branch info not available — skipping",
		}, nil
	}
	if ac.BranchStatus.BehindCount > 0 {
		return Result{
			RuleID: g.ID(), Name: "Branch Up To Date", Passed: false,
			Severity: SeverityStop,
			Message:  fmt.Sprintf("branch is %d commit(s) behind target — rebase and run plan again", ac.BranchStatus.BehindCount),
		}, nil
	}
	return Result{RuleID: g.ID(), Name: "Branch Up To Date", Passed: true, Severity: SeverityStop, Message: "up to date"}, nil
}

// gBranchPoliciesPassing delegates approval, work items, comment resolution and any other
// blocking branch policy to the SCM provider's policy evaluation API.
// Returns SeverityInfo (skip) when PolicyStatus is nil — state transiently unavailable (e.g. GitHub "unknown").
type gBranchPoliciesPassing struct{}

func (g *gBranchPoliciesPassing) ID() string { return "branch_policies_passing" }

func (g *gBranchPoliciesPassing) Check(_ context.Context, ac ApplyContext) (Result, error) {
	if ac.PolicyStatus == nil {
		return Result{
			RuleID: g.ID(), Name: "Branch Policies", Passed: true,
			Severity: SeverityInfo, Message: "policy evaluation not available for this provider — skipping",
		}, nil
	}
	if !ac.PolicyStatus.AllPassing {
		return Result{
			RuleID: g.ID(), Name: "Branch Policies", Passed: false,
			Severity: SeverityStop,
			Message:  fmt.Sprintf("blocking policies not satisfied: %s", strings.Join(ac.PolicyStatus.Failing, ", ")),
		}, nil
	}
	return Result{RuleID: g.ID(), Name: "Branch Policies", Passed: true, Severity: SeverityStop, Message: "all policies satisfied"}, nil
}

// gSHABackendMatch blocks if the pipeline SHA differs from what was registered
// at plan time. Requires a plan registered via POST /api/v1/plans.
type gSHABackendMatch struct{}

func (g *gSHABackendMatch) ID() string { return "sha_backend_match" }

func (g *gSHABackendMatch) Check(_ context.Context, ac ApplyContext) (Result, error) {
	if ac.Deployment == nil {
		return Result{
			RuleID: g.ID(), Name: "SHA Backend Match", Passed: true,
			Severity: SeverityInfo, Message: "no registered plan — skipping backend SHA check",
		}, nil
	}
	if ac.PipelineSHA == "" || ac.Deployment.SourceBranchSHA == "" {
		return Result{
			RuleID: g.ID(), Name: "SHA Backend Match", Passed: true,
			Severity: SeverityInfo, Message: "SHA not available — skipping",
		}, nil
	}
	if ac.PipelineSHA != ac.Deployment.SourceBranchSHA {
		return Result{
			RuleID: g.ID(), Name: "SHA Backend Match", Passed: false,
			Severity: SeverityStop,
			Message:  fmt.Sprintf("pipeline SHA `%s` ≠ registered plan SHA `%s`", ac.PipelineSHA, ac.Deployment.SourceBranchSHA),
		}, nil
	}
	return Result{
		RuleID: g.ID(), Name: "SHA Backend Match", Passed: true,
		Severity: SeverityStop, Message: "pipeline SHA matches registered plan",
	}, nil
}
