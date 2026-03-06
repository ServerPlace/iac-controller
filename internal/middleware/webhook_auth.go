// FILE: pkg/middleware/webhook_auth.go
package middleware

import (
	"net/http"

	"github.com/ServerPlace/iac-controller/internal/webhook/auth"
	"github.com/ServerPlace/iac-controller/pkg/log"
)

// WebhookAuth cria middleware que valida usando o Authenticator fornecido
func WebhookAuth(authenticator auth.Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger := log.FromContext(r.Context())

			if err := authenticator.Authenticate(r); err != nil {
				logger.Warn().
					Err(err).
					Str("path", r.URL.Path).
					Msg("Webhook authentication failed")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			logger.Debug().Str("path", r.URL.Path).Msg("Webhook authenticated")
			next.ServeHTTP(w, r)
		})
	}
}
