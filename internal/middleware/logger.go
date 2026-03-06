package middleware

import (
	"net/http"

	"github.com/ServerPlace/iac-controller/pkg/log"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

func WithLogger(base zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				requestID = uuid.New().String()
			}
			w.Header().Set("X-Request-ID", requestID)
			l := base.With().
				Str("request_id", requestID).
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Logger()
			ctx := log.WithLogger(r.Context(), l)
			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
}
