package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/ServerPlace/iac-controller/internal/admin/auth"
	"github.com/ServerPlace/iac-controller/internal/admin/config"
	"github.com/ServerPlace/iac-controller/pkg/api"
)

type Client struct {
	cfg         *config.Config
	oauthClient *auth.OAuthClient
	httpClient  *http.Client
}

func New(cfg *config.Config) *Client {
	return &Client{
		cfg:         cfg,
		oauthClient: auth.NewOAuthClient(cfg.ClientID, cfg.ClientSecret),
		httpClient:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) getToken() (*auth.StoredToken, error) {
	token, err := auth.LoadToken()
	if err != nil {
		fmt.Println("Sessão expirada ou não encontrada. Iniciando login...")
		token, err = c.oauthClient.Login()
		if err != nil {
			return nil, err
		}
		if err := auth.SaveToken(token); err != nil {
			return nil, err
		}
		fmt.Println("💾 Login realizado com sucesso!")
	}
	return token, nil
}

func (c *Client) RegisterRepository(identifier, provider string) error {
	token, err := c.getToken()
	if err != nil {
		return fmt.Errorf("erro na autenticação: %w", err)
	}

	reqBody, _ := json.Marshal(api.CreateRepositoryRequest{
		Provider:   provider,
		Identifier: identifier,
	})

	req, err := http.NewRequest("POST", c.cfg.BackendURL+"/admin/repositories", bytes.NewBuffer(reqBody))
	if err != nil {
		return fmt.Errorf("erro ao criar request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token.IDToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("erro na conexão: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("servidor retornou erro (%d): %s", resp.StatusCode, string(body))
	}

	var result api.CreateRepositoryResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("erro ao decodificar resposta: %w", err)
	}

	if result.CreatedNew {
		fmt.Println("\n✅ Repositório Registrado com Sucesso!")
	} else {
		fmt.Println("\n✅ Repositório Já Existente")
	}

	fmt.Printf("📂 Provider:     %s\n", result.RepositoryMetadata.SCMProvider)
	fmt.Printf("🆔 ID:           %s\n", result.RepositoryMetadata.ID)
	fmt.Printf("📛 Nome:         %s\n", result.RepositoryMetadata.Name)
	fmt.Printf("🔗 URI:          %s\n", result.RepositoryMetadata.RepoURI)

	if result.RepoSecret != "" {
		fmt.Printf("🔐 Secret (K_r): %s\n", result.RepoSecret)
	}

	if result.Instruction != "" {
		fmt.Printf("\n💡 Instruções:\n%s\n", result.Instruction)
	}

	return nil
}

func (c *Client) Logout() error {
	err := auth.DeleteToken()
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("erro ao remover token: %w", err)
	}
	return nil
}

func (c *Client) Whoami() error {
	token, err := auth.LoadToken()
	if err != nil {
		return fmt.Errorf("nenhuma sessão ativa. Execute 'register-repo' para fazer login")
	}

	payload, err := auth.DecodeJWT(token.IDToken)
	if err != nil {
		return fmt.Errorf("erro ao decodificar token: %w", err)
	}

	fmt.Printf("👤 Usuário logado:\n")
	fmt.Printf("   Email: %s\n", payload["email"])
	fmt.Printf("   Expira em: %s\n", token.Expiry.Format("2006-01-02 15:04:05"))

	return nil
}
