package azure

import (
	"fmt"
	"net/url"
	"strings"
)

// URIComponents contém os componentes extraídos de uma URI do Azure DevOps
type URIComponents struct {
	Organization string // Nome da organização (ex: "myorg")
	Project      string // Nome do projeto (ex: "myproject")
	RepoName     string // Nome do repositório (ex: "myrepo")
}

// ParseURI extrai os componentes de uma URI do Azure DevOps
// Formatos suportados:
// - https://dev.azure.com/org/project/_git/repo
// - https://user@dev.azure.com/org/project/_git/repo
// - https://org.visualstudio.com/project/_git/repo (formato legado)
func ParseURI(uri string) (*URIComponents, error) {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return nil, fmt.Errorf("URI is empty")
	}

	// Remove credenciais da URL se existirem (user@)
	// https://user@dev.azure.com/... -> https://dev.azure.com/...
	if strings.Contains(uri, "@") {
		parts := strings.SplitN(uri, "@", 2)
		if len(parts) == 2 {
			// Reconstrói sem credenciais
			scheme := ""
			if strings.HasPrefix(parts[0], "https://") {
				scheme = "https://"
			} else if strings.HasPrefix(parts[0], "http://") {
				scheme = "http://"
			}
			uri = scheme + parts[1]
		}
	}

	// Parse a URL
	parsedURL, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("invalid URI format: %w", err)
	}

	// Valida que é Azure DevOps
	host := strings.ToLower(parsedURL.Host)
	if !strings.Contains(host, "dev.azure.com") && !strings.Contains(host, "visualstudio.com") {
		return nil, fmt.Errorf("not an Azure DevOps URI (host: %s)", host)
	}

	// Parse do path
	// Formato esperado: /org/project/_git/repo
	path := strings.Trim(parsedURL.Path, "/")
	parts := strings.Split(path, "/")

	// Validações básicas
	if len(parts) < 4 {
		return nil, fmt.Errorf("invalid Azure DevOps URI format (expected: /org/project/_git/repo, got: %s)", parsedURL.Path)
	}

	// Verifica se tem _git no path
	gitIndex := -1
	for i, part := range parts {
		if part == "_git" {
			gitIndex = i
			break
		}
	}

	if gitIndex == -1 {
		return nil, fmt.Errorf("invalid Azure DevOps URI: missing '_git' segment")
	}

	// Extrai componentes
	// parts[0] = org
	// parts[1] = project
	// parts[gitIndex] = _git
	// parts[gitIndex+1] = repo
	if gitIndex < 2 || gitIndex+1 >= len(parts) {
		return nil, fmt.Errorf("invalid Azure DevOps URI structure")
	}

	components := &URIComponents{
		Organization: parts[0],
		Project:      parts[1],
		RepoName:     parts[gitIndex+1],
	}

	// Validações
	if components.Organization == "" {
		return nil, fmt.Errorf("organization not found in URI")
	}
	if components.Project == "" {
		return nil, fmt.Errorf("project not found in URI")
	}
	if components.RepoName == "" {
		return nil, fmt.Errorf("repository name not found in URI")
	}

	return components, nil
}

// BuildURI constrói uma URI do Azure DevOps a partir dos componentes
func BuildURI(org, project, repoName string) string {
	return fmt.Sprintf("https://dev.azure.com/%s/%s/_git/%s", org, project, repoName)
}
