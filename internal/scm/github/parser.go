package github

import (
	"fmt"
	"net/url"
	"strings"
)

// URIComponents contém os componentes extraídos de uma URI do GitHub
type URIComponents struct {
	Owner    string // Nome do owner (user ou organization)
	RepoName string // Nome do repositório
}

// ParseURI extrai os componentes de uma URI do GitHub
// Formatos suportados:
// - https://github.com/owner/repo
// - https://github.com/owner/repo.git
// - git@github.com:owner/repo.git
func ParseURI(uri string) (*URIComponents, error) {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return nil, fmt.Errorf("URI is empty")
	}

	// Handle SSH format: git@github.com:owner/repo.git
	if strings.HasPrefix(uri, "git@") {
		parts := strings.Split(uri, ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid SSH URI format")
		}
		// Remove git@ prefix and .git suffix
		path := strings.TrimSuffix(parts[1], ".git")
		pathParts := strings.Split(path, "/")
		if len(pathParts) != 2 {
			return nil, fmt.Errorf("invalid SSH URI: expected owner/repo")
		}
		return &URIComponents{
			Owner:    pathParts[0],
			RepoName: pathParts[1],
		}, nil
	}

	// Handle HTTPS format
	parsedURL, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("invalid URI format: %w", err)
	}

	// Validate that it's GitHub
	host := strings.ToLower(parsedURL.Host)
	if !strings.Contains(host, "github.com") {
		return nil, fmt.Errorf("not a GitHub URI (host: %s)", host)
	}

	// Parse path: /owner/repo or /owner/repo.git
	path := strings.Trim(parsedURL.Path, "/")
	path = strings.TrimSuffix(path, ".git")

	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid GitHub URI format (expected: /owner/repo, got: %s)", parsedURL.Path)
	}

	return &URIComponents{
		Owner:    parts[0],
		RepoName: parts[1],
	}, nil
}

// BuildURI constrói uma URI do GitHub a partir dos componentes
func BuildURI(owner, repoName string) string {
	return fmt.Sprintf("https://github.com/%s/%s", owner, repoName)
}
