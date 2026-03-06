package github

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseURI(t *testing.T) {
	tests := []struct {
		name      string
		uri       string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{
			name:      "standard HTTPS format",
			uri:       "https://github.com/owner/repo",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantErr:   false,
		},
		{
			name:      "HTTPS with .git",
			uri:       "https://github.com/owner/repo.git",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantErr:   false,
		},
		{
			name:      "SSH format",
			uri:       "git@github.com:owner/repo.git",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantErr:   false,
		},
		{
			name:      "SSH without .git",
			uri:       "git@github.com:owner/repo",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantErr:   false,
		},
		{
			name:      "with trailing slash",
			uri:       "https://github.com/owner/repo/",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantErr:   false,
		},
		{
			name:    "invalid - missing repo",
			uri:     "https://github.com/owner",
			wantErr: true,
		},
		{
			name:    "invalid - empty URI",
			uri:     "",
			wantErr: true,
		},
		{
			name:    "invalid - not GitHub",
			uri:     "https://gitlab.com/owner/repo",
			wantErr: true,
		},
		{
			name:    "invalid SSH format",
			uri:     "git@github.com-owner-repo",
			wantErr: true,
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
			assert.Equal(t, tt.wantOwner, components.Owner)
			assert.Equal(t, tt.wantRepo, components.RepoName)
		})
	}
}

func TestBuildURI(t *testing.T) {
	uri := BuildURI("owner", "repo")
	expected := "https://github.com/owner/repo"
	assert.Equal(t, expected, uri)
}
