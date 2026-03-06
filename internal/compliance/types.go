package compliance

import (
	"context"
	"github.com/ServerPlace/iac-controller/internal/scm"
)

type Severity string

const (
	SeverityStop Severity = "STOP"
	SeverityInfo Severity = "INFO"
)

type Result struct {
	RuleID   string
	Name     string
	Passed   bool
	Severity Severity
	Message  string
}

// Rule define o contrato que os plugins devem implementar
type Rule interface {
	ID() string
	Validate(ctx context.Context, pr *scm.PullRequest) (Result, error)
}

// Factory para instanciar regras via config
type Factory func(config map[string]interface{}) (Rule, error)

// RuleConfig mapeia o YAML
type RuleConfig struct {
	ID      string                 `mapstructure:"id"`
	Enabled bool                   `mapstructure:"enabled"`
	Params  map[string]interface{} `mapstructure:"params"`
}
