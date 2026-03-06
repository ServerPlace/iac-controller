package credentials

import (
	"context"
	"fmt"
	"time"

	"github.com/ServerPlace/iac-controller/pkg/hmac"
	"github.com/ServerPlace/iac-controller/pkg/log"
)

// ValidateHMAC valida a assinatura HMAC de uma request e retorna o repositório
// Atualizado para aceitar ID nativo, URI ou nome (backward compatible)
//
// Exemplo:
//
//	var req api.RegisterPlanRequest
//	repoMeta, err := ValidateHMAC(ctx, repo, secret, req.Persistence, req)

func ValidateHMAC[T hmac.Signable](ctx context.Context, secret string, req T) (bool, error) {
	logger := log.FromContext(ctx)

	// 1. Validação de timestamp (anti-replay)
	reqTime := time.Unix(req.GetTimestamp(), 0)
	if time.Since(reqTime).Abs() > 5*time.Minute {
		logger.Warn().
			Int64("timestamp", req.GetTimestamp()).
			Msg("Request timestamp outside acceptable window")
		return false, fmt.Errorf("request expired")
	}

	// 4. Valida HMAC
	// TODO Remove print of request
	//logger.Debug().Msgf("Req: %v", req)
	valid, err := hmac.Verify([]byte(secret), req)
	if err != nil {
		logger.Error().Err(err).Msg("HMAC verification error")
		return false, fmt.Errorf("signature verification error")
	}

	if !valid {
		return false, fmt.Errorf("invalid signature")
	}

	logger.Info().
		Msg("HMAC validation successful")

	return true, nil
}
