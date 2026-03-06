package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/ServerPlace/iac-controller/pkg/log"
	"google.golang.org/api/idtoken"
)

// OIDCAuth agora é uma Factory Function que recebe a allowlist
func OIDCAuth(allowedEmails, allowedAudiences, allowedAZPs []string) func(http.Handler) http.Handler {
	// Cria um mapa para busca rápida (O(1))
	allowedMap := make(map[string]bool)
	for _, email := range allowedEmails {
		allowedMap[email] = true
	}
	azpMap := make(map[string]bool)
	for _, azp := range allowedAZPs {
		azpMap[azp] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger := log.FromContext(r.Context())

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Unauthorized: missing header", http.StatusUnauthorized)
				return
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token == authHeader {
				http.Error(w, "Unauthorized: invalid format", http.StatusUnauthorized)
				return
			}

			// 1. Valida se o token é do Google
			payload, err := idtoken.Validate(context.Background(), token, "")
			if err != nil {
				logger.Warn().Err(err).Msg("Invalid OIDC token")
				if payload != nil {
					logger.Debug().Err(err).Msgf("Invalid OIDC token. aud %s", payload.Audience)
				}
				http.Error(w, "Forbidden: invalid token", http.StatusForbidden)
				return
			}
			tokenAudience := payload.Audience
			audFound := false
			logger.Debug().Msgf("Checking audience %s for %v", tokenAudience, allowedAudiences)
			for _, allowed := range allowedAudiences {
				if tokenAudience == allowed {
					audFound = true
					break
				}
			}

			if !audFound {
				logger.Warn().Msgf("audience rejeitado: recebido '%s', esperados %v", tokenAudience, allowedAudiences)
				http.Error(w, "Forbidden: invalid audience", http.StatusForbidden)
				return
			}

			// 2. Validação do AZP (Authorized Party) - O NOVO PASSO
			azpInt, ok := payload.Claims["azp"]
			if !ok {
				logger.Warn().Msg("Token missing azp claim")
				http.Error(w, "Forbidden: missing azp", http.StatusForbidden)
				return
			}
			azp, _ := azpInt.(string)

			if !azpMap[azp] {
				logger.Warn().Str("azp", azp).Msg("Unauthorized application/source (azp)")
				logger.Debug().Str("azp", azp).Msgf("Provided azp: %s not in list %v", azpInt, allowedAZPs)

				http.Error(w, "Forbidden: application not authorized", http.StatusForbidden)
				return
			}

			// 2. Extrai o email do token
			emailInt, ok := payload.Claims["email"]
			if !ok {
				logger.Warn().Msg("Token missing email claim")
				http.Error(w, "Forbidden: invalid claims", http.StatusForbidden)
				return
			}
			email, ok := emailInt.(string)
			if !ok {
				http.Error(w, "Forbidden: invalid email claim type", http.StatusForbidden)
				return
			}

			// 3. Validação da Lista de Permissões (O Pulo do Gato)
			if !allowedMap[email] {
				logger.Warn().Str("email", email).Msg("Unauthorized user attempted access")
				http.Error(w, "Forbidden: user not authorized", http.StatusForbidden)
				return
			}

			logger.Info().Str("user", email).Msg("Access granted")
			next.ServeHTTP(w, r)
		})
	}
}
