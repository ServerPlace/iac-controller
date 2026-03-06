package azure

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseURI(t *testing.T) {
	tests := []struct {
		name        string
		uri         string
		wantOrg     string
		wantProject string
		wantRepo    string
		wantErr     bool
	}{
		{
			name:        "standard format",
			uri:         "https://dev.azure.com/myorg/myproject/_git/myrepo",
			wantOrg:     "myorg",
			wantProject: "myproject",
			wantRepo:    "myrepo",
			wantErr:     false,
		},
		{
			name:        "with user credentials",
			uri:         "https://user@dev.azure.com/myorg/myproject/_git/myrepo",
			wantOrg:     "myorg",
			wantProject: "myproject",
			wantRepo:    "myrepo",
			wantErr:     false,
		},
		{
			name:        "with trailing slash",
			uri:         "https://dev.azure.com/myorg/myproject/_git/myrepo/",
			wantOrg:     "myorg",
			wantProject: "myproject",
			wantRepo:    "myrepo",
			wantErr:     false,
		},
		{
			name:    "invalid - missing _git",
			uri:     "https://dev.azure.com/myorg/myproject/myrepo",
			wantErr: true,
		},
		{
			name:    "invalid - too short path",
			uri:     "https://dev.azure.com/myorg",
			wantErr: true,
		},
		{
			name:    "invalid - empty URI",
			uri:     "",
			wantErr: true,
		},
		{
			name:    "invalid - not Azure DevOps",
			uri:     "https://github.com/owner/repo",
			wantErr: true,
		},
		{
			name:        "real example from context",
			uri:         "https://user@dev.azure.com/org/project/_git/repo",
			wantOrg:     "org",
			wantProject: "project",
			wantRepo:    "repo",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			components, err := ParseURI(tt.uri)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, components)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, components)
			assert.Equal(t, tt.wantOrg, components.Organization)
			assert.Equal(t, tt.wantProject, components.Project)
			assert.Equal(t, tt.wantRepo, components.RepoName)
		})
	}
}

func TestBuildURI(t *testing.T) {
	uri := BuildURI("myorg", "myproject", "myrepo")
	expected := "https://dev.azure.com/myorg/myproject/_git/myrepo"
	assert.Equal(t, expected, uri)
}
