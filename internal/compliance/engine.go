package compliance

import (
	"context"
	"fmt"
	"github.com/ServerPlace/iac-controller/internal/scm"
	"strings"
)

type Engine struct {
	rules []Rule
}

func BuildEngine(configs []RuleConfig) (*Engine, error) {
	var activeRules []Rule
	for _, cfg := range configs {
		if !cfg.Enabled {
			continue
		}
		r, err := Instantiate(cfg.ID, cfg.Params)
		if err != nil {
			return nil, err
		}
		activeRules = append(activeRules, r)
	}
	return &Engine{rules: activeRules}, nil
}

func (e *Engine) Evaluate(ctx context.Context, pr *scm.PullRequest) ([]Result, bool) {
	var results []Result
	ok := true
	for _, r := range e.rules {
		res, err := r.Validate(ctx, pr)
		if err != nil {
			res = Result{RuleID: r.ID(), Passed: false, Severity: SeverityStop, Message: err.Error()}
		}
		if !res.Passed && res.Severity == SeverityStop {
			ok = false
		}
		results = append(results, res)
	}
	return results, ok
}

func GenerateReport(results []Result, ok bool) string {
	var sb strings.Builder
	if ok {
		sb.WriteString("### 🛡️ Compliance Check: **PASSED**\n\n")
	} else {
		sb.WriteString("### 🛑 Compliance Check: **BLOCKED**\n\n")
	}
	sb.WriteString("| Rule | Status | Message |\n|---|:---:|---|\n")
	for _, r := range results {
		icon := "✅"
		if !r.Passed {
			if r.Severity == SeverityStop {
				icon = "⛔"
			} else {
				icon = "⚠️"
			}
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s |\n", r.Name, icon, r.Message))
	}
	return sb.String()
}
