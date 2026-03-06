// FILE: internal/webhook/auth/bearer.go
package auth

import (
	"fmt"
	"net/http"
	"strings"
)

type BearerAuthenticator struct {
	token      string
	headerName string
}

func NewBearerAuthenticator(token, headerName string) *BearerAuthenticator {
	return &BearerAuthenticator{
		token:      token,
		headerName: headerName,
	}
}

func (a *BearerAuthenticator) Authenticate(r *http.Request) error {
	receivedToken := r.Header.Get(a.headerName)
	if receivedToken == "" {
		return fmt.Errorf("missing token header: %s", a.headerName)
	}

	receivedToken = strings.TrimPrefix(receivedToken, "Bearer ")

	if receivedToken != a.token {
		return fmt.Errorf("invalid token")
	}

	return nil
}
