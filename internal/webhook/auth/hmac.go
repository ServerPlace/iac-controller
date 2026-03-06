// FILE: internal/webhook/auth/hmac.go
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type HMACAuthenticator struct {
	secret        []byte
	headerName    string
	signatureType string
}

func NewHMACAuthenticator(secret, headerName, sigType string) *HMACAuthenticator {
	return &HMACAuthenticator{
		secret:        []byte(secret),
		headerName:    headerName,
		signatureType: sigType,
	}
}

func (a *HMACAuthenticator) Authenticate(r *http.Request) error {
	signature := r.Header.Get(a.headerName)
	if signature == "" {
		return fmt.Errorf("missing signature header: %s", a.headerName)
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("failed to read body: %w", err)
	}
	defer func() {
		r.Body = io.NopCloser(strings.NewReader(string(body)))
	}()

	mac := hmac.New(sha256.New, a.secret)
	mac.Write(body)
	expectedMAC := mac.Sum(nil)
	expectedSig := fmt.Sprintf("%s=%s", a.signatureType, hex.EncodeToString(expectedMAC))

	if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
		return fmt.Errorf("invalid signature")
	}

	return nil
}
