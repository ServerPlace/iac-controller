package scm

import (
	"fmt"
	"strings"

	"github.com/ServerPlace/iac-controller/internal/core/model"
)

// DetectProvider tenta detectar o provider SCM baseado na URI
// Suporta Azure DevOps, GitHub e GitLab na nuvem
// Para instalações self-hosted, o provider deve ser informado explicitamente
func DetectProvider(uri string) (model.SCMProvider, error) {
	uri = strings.ToLower(strings.TrimSpace(uri))

	// Azure DevOps (cloud)
	// Padrões:
	// - https://dev.azure.com/org/project/_git/repo
	// - https://user@dev.azure.com/org/project/_git/repo
	if strings.Contains(uri, "dev.azure.com") {
		return model.SCMProviderAzure, nil
	}

	// GitHub (cloud)
	// Padrões:
	// - https://github.com/owner/repo
	// - https://github.com/owner/repo.git
	// - git@github.com:owner/repo.git
	if strings.Contains(uri, "github.com") {
		return model.SCMProviderGitHub, nil
	}

	// GitLab (cloud)
	// Padrões:
	// - https://gitlab.com/group/project
	// - https://gitlab.com/group/subgroup/project
	// - git@gitlab.com:group/project.git
	if strings.Contains(uri, "gitlab.com") {
		return model.SCMProviderGitLab, nil
	}

	// Não conseguiu detectar
	return "", fmt.Errorf("cannot detect SCM provider from URI: %s (supported: dev.azure.com, github.com, gitlab.com)", uri)
}
