package cmd

import (
	"fmt"
	"os"

	"github.com/ServerPlace/iac-controller/internal/admin/client"
	"github.com/ServerPlace/iac-controller/internal/admin/config"
	"github.com/spf13/cobra"
)

var ctrl *client.Client

var rootCmd = &cobra.Command{
	Use:   "iac-admin",
	Short: "CLI para gerenciamento do IaC Controller",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Name() == "init" || cmd.Name() == "help" {
			return nil
		}
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("❌ Erro ao carregar configuração: %v\n💡 Execute 'iac-admin init' para configurar", err)
		}
		ctrl = client.New(cfg)
		return nil
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
