package compliance

import (
	"context"
	"fmt"

	"github.com/ServerPlace/iac-controller/internal/core/model"
	"github.com/ServerPlace/iac-controller/internal/scm"
)

// ApplyContext holds all data available to apply gates.
// Deployment may be nil if no plan was registered for this PR.
// BranchStatus may be nil if source/target branches were not provided.
// PolicyStatus may be nil if the SCM provider does not support policy evaluations (e.g. GitHub).
type ApplyContext struct {
	PR           *scm.PullRequest
	Deployment   *model.Deployment   // nil → sha_backend_match degrades to SeverityInfo
	BranchStatus *scm.GitBranchStats // nil → branch_up_to_date skips (SeverityInfo)
	PolicyStatus *scm.PRPolicyStatus // nil → branch_policies_passing skips (SeverityInfo)
	PipelineSHA  string              // req.SourceBranchSHA sent by the pipeline
}

// ApplyGate is a configurable check run before issuing an apply credential.
type ApplyGate interface {
	ID() string
	Check(ctx context.Context, ac ApplyContext) (Result, error)
}

// ApplyEngine evaluates all configured gates in order.
type ApplyEngine struct {
	gates []ApplyGate
}

// BuildApplyEngine constructs an engine from config.
// If configs is empty, DefaultApplyGateConfigs() is used.
func BuildApplyEngine(configs []RuleConfig) (*ApplyEngine, error) {
	if len(configs) == 0 {
		configs = DefaultApplyGateConfigs()
	}
	var active []ApplyGate
	for _, cfg := range configs {
		if !cfg.Enabled {
			continue
		}
		g, err := newApplyGate(cfg.ID)
		if err != nil {
			return nil, err
		}
		active = append(active, g)
	}
	return &ApplyEngine{gates: active}, nil
}

// Check runs all gates and returns results + overall pass/fail.
// A gate with SeverityStop that does not pass causes ok=false.
func (e *ApplyEngine) Check(ctx context.Context, ac ApplyContext) ([]Result, bool) {
	var results []Result
	ok := true
	for _, g := range e.gates {
		res, err := g.Check(ctx, ac)
		if err != nil {
			res = Result{RuleID: g.ID(), Name: g.ID(), Passed: false, Severity: SeverityStop, Message: err.Error()}
		}
		if !res.Passed && res.Severity == SeverityStop {
			ok = false
		}
		results = append(results, res)
	}
	return results, ok
}

// DefaultApplyGateConfigs returns only the gates that match current behaviour.
// pr_approved and sha_backend_match must be explicitly enabled in config
// to avoid unintentional behaviour changes on existing deployments.
func DefaultApplyGateConfigs() []RuleConfig {
	return []RuleConfig{
		{ID: "sha_stale", Enabled: true},
		{ID: "branch_up_to_date", Enabled: true},
		{ID: "sha_backend_match", Enabled: true},
		{ID: "branch_policies_passing", Enabled: true},
	}
}

// newApplyGate resolves a gate by ID. No global registry — extensibility is
// via adding cases here and a new type in apply_gates.go.
func newApplyGate(id string) (ApplyGate, error) {
	switch id {
	case "sha_stale":
		return &gSHAStale{}, nil
	case "branch_up_to_date":
		return &gBranchUpToDate{}, nil
	case "sha_backend_match":
		return &gSHABackendMatch{}, nil
	case "branch_policies_passing":
		return &gBranchPoliciesPassing{}, nil
	default:
		return nil, fmt.Errorf("unknown apply gate %q", id)
	}
}
