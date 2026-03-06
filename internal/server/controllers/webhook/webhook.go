package webhook

import (
	"context"
	"net/http"
	"strings"

	"github.com/ServerPlace/iac-controller/internal/service"
	"github.com/ServerPlace/iac-controller/internal/webhook"
	"github.com/ServerPlace/iac-controller/pkg/log"
)

// WebhookController handles webhook events from SCM providers
// This is the Atlantis-style controller (handler as struct method)
type WebhookController struct {
	Processor webhook.Processor
	Service   *service.DeploymentService
	Provider  string
}

// NewWebhookController creates a new webhook controller
func NewWebhookController(
	processor webhook.Processor,
	deployService *service.DeploymentService,
	provider string,
) *WebhookController {
	return &WebhookController{
		Processor: processor,
		Service:   deployService,
		Provider:  provider,
	}
}

// ServeHTTP implements http.Handler interface
// NOTA: Este é o código EXATO do handler atual, só mudou o tipo
func (c *WebhookController) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.FromContext(ctx)

	// 1. Parse
	result, err := c.Processor.Parse(r)
	if err != nil {
		logger.Error().
			Err(err).
			Str("provider", c.Provider).
			Msg("Parse failed")
		http.Error(w, "parse error", http.StatusBadRequest)
		return
	}

	// 2. Log estruturado
	logger.Info().
		Str("provider", c.Provider).
		Str("type", string(result.Event.Type)).
		Str("repo", result.Event.Repo).
		Int("pr", result.Event.PRNumber).
		Bool("should_queue", result.ShouldQueue).
		Msg("Webhook processed")

	// 3. Roteamento
	if result.ShouldQueue {
		c.routeEvent(ctx, result.Event)
	}

	// 4. Responder
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(result.Message))
}

// routeEvent recebe apenas contexto (logger vem de dentro)
func (c *WebhookController) routeEvent(ctx context.Context, event *webhook.NormalizedEvent) {
	logger := log.FromContext(ctx)

	switch event.Type {
	case webhook.EventTypeComment:
		if strings.HasPrefix(strings.TrimSpace(event.Body), "/apply") {
			logger.Warn().Msg("/apply received but disabled — use the pipeline-triggered flow")
		}

	case webhook.EventTypePRUpdate:
		logger.Info().Msg("PR updated, invalidating stale jobs")
		// TODO: Implementar invalidação

	case webhook.EventTypePRClosed:
		logger.Info().Bool("merged", event.IsMerged).Msg("Cleaning up PR resources")

		// NOVO: Release automático de locks quando PR fecha
		go func() {
			bgCtx := context.Background()
			bgCtx = log.WithLogger(bgCtx, logger)

			if err := c.Service.ReleasePRLocks(bgCtx, event.Repo, event.PRNumber); err != nil {
				logger.Error().Err(err).Msg("Failed to release locks on PR close")
			} else {
				logger.Info().Msg("Successfully released all locks for closed PR")
			}
		}()

	case webhook.EventTypePRApproved:
		logger.Info().Msg("PR approved, updating state")
		// TODO: Atualizar FSM
	}
}
