package auth

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/pkg/browser"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type OAuthClient struct {
	config *oauth2.Config
}

func NewOAuthClient(clientID, clientSecret string) *OAuthClient {
	return &OAuthClient{
		config: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint:     google.Endpoint,
			RedirectURL:  "http://127.0.0.1:8080",
			Scopes:       []string{"openid", "email"},
		},
	}
}

func (o *OAuthClient) Login() (*StoredToken, error) {
	codeChan := make(chan string)
	errChan := make(chan error)

	mux := http.NewServeMux()
	server := &http.Server{
		Addr:    "127.0.0.1:8080",
		Handler: mux,
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			errorDesc := r.URL.Query().Get("error")
			if errorDesc != "" {
				w.Write([]byte(fmt.Sprintf("❌ Erro: %s", errorDesc)))
				errChan <- fmt.Errorf("OAuth error: %s", errorDesc)
			} else {
				w.Write([]byte("❌ Código não recebido"))
				errChan <- fmt.Errorf("código ausente")
			}
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`
			<!DOCTYPE html>
			<html>
			<head><meta charset="UTF-8"><title>Autenticação Concluída</title></head>
			<body style="font-family: Arial; text-align: center; padding: 50px;">
				<h1 style="color: #4CAF50;">✅ Login realizado!</h1>
				<p>Você pode fechar esta janela e voltar ao terminal.</p>
			</body>
			</html>
		`))
		codeChan <- code
	})

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	url := o.config.AuthCodeURL(
		"state-token",
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "select_account consent"),
	)

	fmt.Printf("🌐 Abrindo navegador para autenticação...\n")
	fmt.Printf("💡 Selecione a conta Google desejada\n\n")
	browser.OpenURL(url)

	select {
	case code := <-codeChan:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(ctx)

		token, err := o.config.Exchange(context.Background(), code)
		if err != nil {
			return nil, fmt.Errorf("erro na troca do código: %w", err)
		}

		idToken, ok := token.Extra("id_token").(string)
		if !ok {
			return nil, fmt.Errorf("id_token não retornado")
		}

		return &StoredToken{
			AccessToken:  token.AccessToken,
			RefreshToken: token.RefreshToken,
			IDToken:      idToken,
			TokenType:    token.TokenType,
			Expiry:       token.Expiry,
		}, nil

	case err := <-errChan:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(ctx)
		return nil, err

	case <-time.After(3 * time.Minute):
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(ctx)
		return nil, fmt.Errorf("timeout aguardando login")
	}
}
