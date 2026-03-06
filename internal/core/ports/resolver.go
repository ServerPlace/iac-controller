package ports

import (
	"context"
	"fmt"
	"github.com/ServerPlace/iac-controller/internal/core/model"
	"github.com/ServerPlace/iac-controller/pkg/log"
	"net/url"
	"strings"
)

// ResolveManagedRepo tenta encontrar um repositório usando diferentes estratégias
// Aceita: ID nativo, URI completa, ou nome (legacy)
// Útil durante período de migração onde clientes podem enviar formatos diferentes
//
// Ordem de tentativa:
// 1. ID nativo (busca direta - mais rápido)
// 2. URI completa (query por repo_uri)
// 3. Nome (legacy - query por name)
func ResolveManagedRepo(ctx context.Context, repoStorage Persistence, managedRepoIdentifier string) (*model.RepositoryMetadata, error) {
	logger := log.FromContext(ctx)
	if managedRepoIdentifier == "" {
		return nil, fmt.Errorf("managedRepoIdentifier is empty")
	}
	defer func() {
		logger.Trace().Msgf("managedRepoIdentifier is %s", managedRepoIdentifier)
	}()
	// 1. Tenta buscar por ID (assume que é ID nativo)
	managedRepoMetadata, err := repoStorage.GetRepositoryByID(ctx, managedRepoIdentifier)
	if err == nil {
		return managedRepoMetadata, nil
	}

	// 2. Tenta buscar por URI
	managedRepoMetadata, err = repoStorage.GetRepositoryByURI(ctx, normalizeIdentifier(managedRepoIdentifier))
	if err == nil {
		return managedRepoMetadata, nil
	}

	// 3. Tenta buscar por Name (legacy)
	managedRepoMetadata, err = repoStorage.GetRepositoryByName(ctx, managedRepoIdentifier)
	if err == nil {
		return managedRepoMetadata, nil
	}

	// Não encontrou de forma alguma
	return nil, fmt.Errorf("instances repository not found with Identifier: %s (tried: ID, URI, Name)", managedRepoIdentifier)
}

func normalizeIdentifier(identifier string) string {
	// Se não parece uma URL, retorna como está (pode ser um GUID ou nome)
	if !strings.HasPrefix(identifier, "http://") && !strings.HasPrefix(identifier, "https://") {
		return identifier
	}

	// Parse a URL
	u, err := url.Parse(identifier)
	if err != nil {
		// Se falhar o parse, retorna como está
		return identifier
	}

	// Remove o username (user info)
	if u.User != nil {
		u.User = nil
	}

	return u.String()
}
