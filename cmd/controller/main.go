package main

import (
	"context"
	"github.com/ServerPlace/iac-controller/pkg/version"
	"github.com/rs/zerolog"
	"os"
	"time"

	"github.com/ServerPlace/iac-controller/internal/config"
	"github.com/ServerPlace/iac-controller/internal/server"
	"github.com/ServerPlace/iac-controller/pkg/log"
)

func main() {
	logger := log.New(log.Setup())
	zerolog.LevelFieldName = "severity"
	// Use uppercase for severity values (INFO, ERROR, etc.) as expected by GCP
	zerolog.TimestampFieldName = "time"

	// Timeout curto apenas para inicialização (ler configs, conectar bancos)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "/app/config"
	}
	// 1. Carrega Config (Resolve Secrets aqui)
	cfg, err := config.Load(ctx, configPath)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to load config")
	}

	// 2. Cria Server (NOVO - padrão Atlantis)
	// Nota: NewServer já inicializa todas as dependências e controllers
	srv, err := server.NewServer(cfg, logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create server")
		os.Exit(1)
	}

	// 3. Start Server (NOVO - método do próprio Server)
	// Nota: ListenAndServe já aplica logging middleware e configura rotas
	logger.Info().Msgf("iac-controller %s (build %s) starting on port %s.", version.Version, version.BuildTime, cfg.Port)
	if err := srv.ListenAndServe(); err != nil {
		logger.Fatal().Err(err).Msg("server crashed")
		os.Exit(1)
	}
}
