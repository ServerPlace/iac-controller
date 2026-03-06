package scm

import (
	"testing"

	"github.com/ServerPlace/iac-controller/internal/core/model"
	"github.com/stretchr/testify/assert"
)

func TestDetectProvider(t *testing.T) {
	tests := []struct {
		name         string
		uri          string
		wantProvider model.SCMProvider
		wantErr      bool
	}{
		{
			name:         "Azure DevOps - standard",
			uri:          "https://dev.azure.com/myorg/myproject/_git/myrepo",
			wantProvider: model.SCMProviderAzure,
			wantErr:      false,
		},
		{
			name:         "Azure DevOps - with credentials",
			uri:          "https://user@dev.azure.com/myorg/myproject/_git/myrepo",
			wantProvider: model.SCMProviderAzure,
			wantErr:      false,
		},
		{
			name:         "Azure DevOps - case insensitive",
			uri:          "HTTPS://DEV.AZURE.COM/myorg/myproject/_git/myrepo",
			wantProvider: model.SCMProviderAzure,
			wantErr:      false,
		},
		{
			name:         "GitHub - HTTPS",
			uri:          "https://github.com/owner/repo",
			wantProvider: model.SCMProviderGitHub,
			wantErr:      false,
		},
		{
			name:         "GitHub - with .git",
			uri:          "https://github.com/owner/repo.git",
			wantProvider: model.SCMProviderGitHub,
			wantErr:      false,
		},
		{
			name:         "GitHub - SSH",
			uri:          "git@github.com:owner/repo.git",
			wantProvider: model.SCMProviderGitHub,
			wantErr:      false,
		},
		{
			name:         "GitLab - HTTPS",
			uri:          "https://gitlab.com/group/project",
			wantProvider: model.SCMProviderGitLab,
			wantErr:      false,
		},
		{
			name:         "GitLab - with subgroup",
			uri:          "https://gitlab.com/group/subgroup/project",
			wantProvider: model.SCMProviderGitLab,
			wantErr:      false,
		},
		{
			name:         "GitLab - SSH",
			uri:          "git@gitlab.com:group/project.git",
			wantProvider: model.SCMProviderGitLab,
			wantErr:      false,
		},
		{
			name:    "Unknown provider",
			uri:     "https://bitbucket.org/team/repo",
			wantErr: true,
		},
		{
			name:    "Self-hosted - needs explicit provider",
			uri:     "https://git.company.com/team/repo",
			wantErr: true,
		},
		{
			name:    "Empty URI",
			uri:     "",
			wantErr: true,
		},
		{
			name:    "Invalid URI",
			uri:     "not-a-valid-uri",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := DetectProvider(tt.uri)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Empty(t, provider)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.wantProvider, provider)
		})
	}
}
