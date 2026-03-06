package github

import (
	"fmt"
	"net/http"

	"github.com/ServerPlace/iac-controller/internal/scm"
	"github.com/google/go-github/v57/github"
)

// ParseWebhook valida a assinatura HMAC e converte para o modelo de domínio
func (g *GithubClient) ParseWebhook(r *http.Request) (*scm.WebhookEvent, error) {
	// 1. Validação de Segurança (HMAC SHA-256)
	// A biblioteca do Google já faz a leitura do Body e validação contra o segredo
	payload, err := github.ValidatePayload(r, g.webhookSecret)
	if err != nil {
		return nil, fmt.Errorf("invalid signature: %w", err)
	}

	// 2. Parse do Evento
	event, err := github.ParseWebHook(github.WebHookType(r), payload)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	// 3. Normalização para o Domínio
	switch e := event.(type) {
	case *github.IssueCommentEvent:
		// Só nos interessa comentários em PRs e quando são criados
		if e.GetAction() == "created" && e.GetIssue().IsPullRequest() {
			return &scm.WebhookEvent{
				Type:     scm.EventComment,
				Repo:     e.GetRepo().GetName(), // Ou FullName, dependendo de como você usa no Service
				PRNumber: e.GetIssue().GetNumber(),
				Sender:   e.GetSender().GetLogin(),
				Body:     e.GetComment().GetBody(),
			}, nil
		}

	case *github.PingEvent:
		return &scm.WebhookEvent{
			Type: scm.EventPing,
			Repo: e.GetRepo().GetName(),
		}, nil
	}

	// Retorna nil, nil para eventos ignorados (ex: star, fork, push)
	return nil, nil
}
