// FILE: internal/webhook/auth/basic.go
package auth

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
)

type BasicAuthenticator struct {
	username string
	password string
}

func NewBasicAuthenticator(username, password string) *BasicAuthenticator {
	return &BasicAuthenticator{
		username: username,
		password: password,
	}
}

func (a *BasicAuthenticator) Authenticate(r *http.Request) error {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return fmt.Errorf("missing authorization header")
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Basic" {
		return fmt.Errorf("invalid auth header format")
	}

	decoded, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("invalid base64 encoding: %w", err)
	}

	creds := strings.SplitN(string(decoded), ":", 2)
	if len(creds) != 2 {
		return fmt.Errorf("invalid credentials format")
	}

	if creds[0] != a.username || creds[1] != a.password {
		return fmt.Errorf("invalid credentials")
	}

	return nil
}
