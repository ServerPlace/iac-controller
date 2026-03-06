package auth

import "net/http"

// Authenticator valida a autenticidade de um webhook
type Authenticator interface {
	Authenticate(r *http.Request) error
}
