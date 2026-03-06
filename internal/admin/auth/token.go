package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type StoredToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	IDToken      string    `json:"id_token"`
	TokenType    string    `json:"token_type"`
	Expiry       time.Time `json:"expiry"`
}

func GetTokenPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".iac-controller", "token.json")
}

func SaveToken(token *StoredToken) error {
	path := GetTokenPath()
	os.MkdirAll(filepath.Dir(path), 0700)

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("erro ao salvar token: %w", err)
	}
	defer f.Close()

	if err := json.NewEncoder(f).Encode(token); err != nil {
		return fmt.Errorf("erro ao codificar token: %w", err)
	}

	return nil
}

func LoadToken() (*StoredToken, error) {
	path := GetTokenPath()
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("token não encontrado")
	}
	defer f.Close()

	var t StoredToken
	if err := json.NewDecoder(f).Decode(&t); err != nil {
		return nil, fmt.Errorf("erro ao ler token")
	}

	if time.Now().Add(5 * time.Minute).After(t.Expiry) {
		return nil, fmt.Errorf("token expirado")
	}

	if t.IDToken == "" {
		return nil, fmt.Errorf("id_token ausente")
	}

	return &t, nil
}

func DeleteToken() error {
	return os.Remove(GetTokenPath())
}

func DecodeJWT(token string) (map[string]interface{}, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("token inválido")
	}

	payload := parts[1]
	// Add padding
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}

	data, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}
