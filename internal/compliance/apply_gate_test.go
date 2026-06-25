package compliance

import (
	"context"
	"errors"
	"testing"

	"github.com/ServerPlace/iac-controller/internal/core/model"
	"github.com/ServerPlace/iac-controller/internal/scm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Engine tests ---

func TestBuildApplyEngine_DefaultsWhenEmpty(t *testing.T) {
	e, err := BuildApplyEngine(nil)
	require.NoError(t, err)
	assert.Len(t, e.gates, 4, "defaults: sha_stale + branch_up_to_date + sha_backend_match + branch_policies_passing")
}

func TestBuildApplyEngine_UnknownGateReturnsError(t *testing.T) {
	_, err := BuildApplyEngine([]RuleConfig{{ID: "does_not_exist", Enabled: true}})
	assert.ErrorContains(t, err, "does_not_exist")
}

func TestBuildApplyEngine_DisabledGatesSkipped(t *testing.T) {
	configs := []RuleConfig{
		{ID: "sha_stale", Enabled: true},
		{ID: "pr_approved", Enabled: false},
	}
	e, err := BuildApplyEngine(configs)
	require.NoError(t, err)
	assert.Len(t, e.gates, 1)
}

func TestApplyEngine_AllPass(t *testing.T) {
	e, _ := BuildApplyEngine([]RuleConfig{{ID: "sha_stale", Enabled: true}})
	ac := ApplyContext{
		PR:          &scm.PullRequest{HeadSHA: "abc"},
		PipelineSHA: "abc",
	}
	_, ok := e.Check(context.Background(), ac)
	assert.True(t, ok)
}

func TestApplyEngine_StopGateFails(t *testing.T) {
	e, _ := BuildApplyEngine([]RuleConfig{{ID: "sha_stale", Enabled: true}})
	ac := ApplyContext{PR: &scm.PullRequest{HeadSHA: "bbb"}, PipelineSHA: "aaa"}
	results, ok := e.Check(context.Background(), ac)
	assert.False(t, ok)
	assert.Len(t, results, 1)
	assert.False(t, results[0].Passed)
}

func TestApplyEngine_InfoGateDoesNotBlock(t *testing.T) {
	e, _ := BuildApplyEngine([]RuleConfig{{ID: "sha_stale", Enabled: true}})
	// Empty SHAs → Info → does not block
	ac := ApplyContext{PR: &scm.PullRequest{}}
	_, ok := e.Check(context.Background(), ac)
	assert.True(t, ok)
}

func TestApplyEngine_GateErrorCountsAsStop(t *testing.T) {
	e := &ApplyEngine{gates: []ApplyGate{&errGate{}}}
	ac := ApplyContext{PR: &scm.PullRequest{}}
	_, ok := e.Check(context.Background(), ac)
	assert.False(t, ok)
}

// --- Gate unit tests ---

func TestGSHAStale_Blocks(t *testing.T) {
	g := &gSHAStale{}
	res, err := g.Check(context.Background(), ApplyContext{
		PR:          &scm.PullRequest{HeadSHA: "bbb"},
		PipelineSHA: "aaa",
	})
	require.NoError(t, err)
	assert.False(t, res.Passed)
	assert.Equal(t, SeverityStop, res.Severity)
}

func TestGSHAStale_Pass(t *testing.T) {
	g := &gSHAStale{}
	res, _ := g.Check(context.Background(), ApplyContext{
		PR:          &scm.PullRequest{HeadSHA: "abc"},
		PipelineSHA: "abc",
	})
	assert.True(t, res.Passed)
}

func TestGSHAStale_EmptySHASkips(t *testing.T) {
	g := &gSHAStale{}
	res, _ := g.Check(context.Background(), ApplyContext{PR: &scm.PullRequest{}})
	assert.True(t, res.Passed)
	assert.Equal(t, SeverityInfo, res.Severity)
}

func TestGBranchUpToDate_Blocks(t *testing.T) {
	g := &gBranchUpToDate{}
	res, _ := g.Check(context.Background(), ApplyContext{
		PR:           &scm.PullRequest{},
		BranchStatus: &scm.GitBranchStats{BehindCount: 3},
	})
	assert.False(t, res.Passed)
	assert.Equal(t, SeverityStop, res.Severity)
}

func TestGBranchUpToDate_NilStatusSkips(t *testing.T) {
	g := &gBranchUpToDate{}
	res, _ := g.Check(context.Background(), ApplyContext{PR: &scm.PullRequest{}})
	assert.True(t, res.Passed)
	assert.Equal(t, SeverityInfo, res.Severity)
}

func TestGBranchUpToDate_Pass(t *testing.T) {
	g := &gBranchUpToDate{}
	res, _ := g.Check(context.Background(), ApplyContext{
		PR:           &scm.PullRequest{},
		BranchStatus: &scm.GitBranchStats{BehindCount: 0},
	})
	assert.True(t, res.Passed)
}

func TestGSHABackendMatch_Blocks(t *testing.T) {
	g := &gSHABackendMatch{}
	res, _ := g.Check(context.Background(), ApplyContext{
		PR:          &scm.PullRequest{},
		Deployment:  &model.Deployment{SourceBranchSHA: "registered"},
		PipelineSHA: "different",
	})
	assert.False(t, res.Passed)
	assert.Equal(t, SeverityStop, res.Severity)
}

func TestGSHABackendMatch_NilDeploymentSkips(t *testing.T) {
	g := &gSHABackendMatch{}
	res, _ := g.Check(context.Background(), ApplyContext{
		PR:          &scm.PullRequest{},
		PipelineSHA: "abc",
	})
	assert.True(t, res.Passed)
	assert.Equal(t, SeverityInfo, res.Severity)
}

func TestGSHABackendMatch_Pass(t *testing.T) {
	g := &gSHABackendMatch{}
	res, _ := g.Check(context.Background(), ApplyContext{
		PR:          &scm.PullRequest{},
		Deployment:  &model.Deployment{SourceBranchSHA: "abc"},
		PipelineSHA: "abc",
	})
	assert.True(t, res.Passed)
}

func TestGBranchPoliciesPassing_Blocks(t *testing.T) {
	g := &gBranchPoliciesPassing{}
	res, _ := g.Check(context.Background(), ApplyContext{
		PR: &scm.PullRequest{},
		PolicyStatus: &scm.PRPolicyStatus{
			AllPassing: false,
			Failing:    []string{"Minimum number of reviewers", "Linked work items"},
		},
	})
	assert.False(t, res.Passed)
	assert.Equal(t, SeverityStop, res.Severity)
	assert.Contains(t, res.Message, "Minimum number of reviewers")
}

func TestGBranchPoliciesPassing_NilStatusSkips(t *testing.T) {
	g := &gBranchPoliciesPassing{}
	res, _ := g.Check(context.Background(), ApplyContext{PR: &scm.PullRequest{}})
	assert.True(t, res.Passed)
	assert.Equal(t, SeverityInfo, res.Severity)
}

func TestGBranchPoliciesPassing_Pass(t *testing.T) {
	g := &gBranchPoliciesPassing{}
	res, _ := g.Check(context.Background(), ApplyContext{
		PR:           &scm.PullRequest{},
		PolicyStatus: &scm.PRPolicyStatus{AllPassing: true},
	})
	assert.True(t, res.Passed)
}

// errGate is a stub that always returns an error, used to test engine error handling.
type errGate struct{}

func (e *errGate) ID() string { return "err_gate" }
func (e *errGate) Check(_ context.Context, _ ApplyContext) (Result, error) {
	return Result{}, errors.New("simulated gate error")
}
