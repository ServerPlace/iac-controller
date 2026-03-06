package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/ServerPlace/iac-controller/internal/core/model"
	"github.com/ServerPlace/iac-controller/internal/core/ports"
	"github.com/ServerPlace/iac-controller/internal/credentials"
	"github.com/ServerPlace/iac-controller/pkg/hmac"
	"github.com/ServerPlace/iac-controller/pkg/log"
)

type ctxRepoKey struct{}

// RepoFrom returns the *model.RepositoryMetadata injected by HMACAuth.
// Returns nil if the middleware was not applied (e.g. in LOCAL_DEV without auth).
func RepoFrom(ctx context.Context) *model.RepositoryMetadata {
	repo, _ := ctx.Value(ctxRepoKey{}).(*model.RepositoryMetadata)
	return repo
}

// WithRepo injects repo into ctx. Intended for use in tests.
func WithRepo(ctx context.Context, repo *model.RepositoryMetadata) context.Context {
	return context.WithValue(ctx, ctxRepoKey{}, repo)
}

// HMACAuth returns a middleware that:
//  1. Reads and decodes the JSON body into T.
//  2. Resolves the repository identified by getRepo(req).
//  3. Validates the HMAC signature (timestamp check included).
//  4. Injects the resolved *model.RepositoryMetadata into the request context.
//  5. Restores r.Body so the next handler can decode it again.
func HMACAuth[T hmac.Signable](
	getRepo func(T) string,
	ns credentials.KeyNamespace,
	persistence ports.Persistence,
	masterKey string,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			logger := log.FromContext(ctx)

			// 1. Read body and restore it so the next handler can decode it too.
			body, err := io.ReadAll(r.Body)
			if err != nil {
				logger.Err(err).Msg("Failed to read request body")
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))

			// 2. Decode into the full typed request (required for correct HMAC payload).
			var req T
			if err := json.Unmarshal(body, &req); err != nil {
				logger.Err(err).Msg("Failed to parse request body")
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}

			// 3. Resolve repository and derive HMAC key.
			repo, err := ports.ResolveManagedRepo(ctx, persistence, getRepo(req))
			if err != nil {
				logger.Warn().Err(err).Msg("Unknown repository")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			params := credentials.NewDerivationParams(repo, credentials.WithNameSpace(ns))
			key, err := credentials.DeriveRepoKeys(masterKey, *params)
			if err != nil {
				logger.Err(err).Msg("Key derivation failed")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			// 4. Validate HMAC (also checks timestamp internally).
			if _, err := credentials.ValidateHMAC(ctx, key, req); err != nil {
				logger.Warn().Err(err).Msg("HMAC validation failed")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			// 5. Inject resolved repo and call next handler.
			ctx = context.WithValue(ctx, ctxRepoKey{}, repo)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
