// FILE: internal/webhook/handler.go
package webhook

import (
	"context"
	"net/http"
	"strings"

	"github.com/ServerPlace/iac-controller/internal/service"
	"github.com/ServerPlace/iac-controller/pkg/log"
)

type Handler struct {
	Processor Processor
	Service   *service.DeploymentService
	Provider  string
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := log.FromContext(ctx)

	// 1. Parse
	result, err := h.Processor.Parse(r)
	if err != nil {
		logger.Error().
			Err(err).
			Str("provider", h.Provider).
			Msg("Parse failed")
		http.Error(w, "parse error", http.StatusBadRequest)
		return
	}

	// 2. Log estruturado
	logger.Info().
		Str("provider", h.Provider).
		Str("type", string(result.Event.Type)).
		Str("repo", result.Event.Repo).
		Int("pr", result.Event.PRNumber).
		Bool("should_queue", result.ShouldQueue).
		Msg("Webhook processed")

	// 3. Roteamento
	if result.ShouldQueue {
		h.routeEvent(ctx, result.Event) // ← SEM logger como parâmetro
	}

	// 4. Responder
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(result.Message))
}

// routeEvent recebe apenas contexto (logger vem de dentro)
func (h *Handler) routeEvent(ctx context.Context, event *NormalizedEvent) {
	logger := log.FromContext(ctx) // ← Pega do contexto aqui

	switch event.Type {
	case EventTypeComment:
		if strings.HasPrefix(strings.TrimSpace(event.Body), "/apply") {
			logger.Info().Msg("Triggering apply workflow")
			go func() {
				bgCtx := context.Background()
				// IMPORTANTE: Propaga o logger para o contexto background
				bgCtx = log.WithLogger(bgCtx, logger)

				if err := h.Service.RunApply(bgCtx, event.Repo, event.PRNumber, event.Sender); err != nil {
					logger.Error().Err(err).Msg("Apply failed")
				}
			}()
		}

	case EventTypePRUpdate:
		logger.Info().Msg("PR updated, invalidating stale jobs")
		// TODO: Implementar invalidação

	case EventTypePRClosed:
		logger.Info().Bool("merged", event.IsMerged).Msg("Cleaning up PR resources")
		// TODO: Liberar locks

	case EventTypePRApproved:
		logger.Info().Msg("PR approved, updating state")
		// TODO: Atualizar FSM
	}
}
