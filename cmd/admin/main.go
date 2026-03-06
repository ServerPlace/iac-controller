package main

import (
	"fmt"
	"os"

	"github.com/ServerPlace/iac-controller/internal/admin/client"
	"github.com/ServerPlace/iac-controller/internal/admin/config"
	"github.com/spf13/cobra"
)

func main() {
	// 1. Definimos o cliente como uma variável que será preenchida depois
	var ctrl *client.Client

	var rootCmd = &cobra.Command{
		Use:   "iac-admin",
		Short: "CLI para gerenciamento do IaC Controller",
		// O PersistentPreRunE roda antes de qualquer comando filho
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// 2. Se o comando for 'init' ou 'help', não tentamos carregar o config
			if cmd.Name() == "init" || cmd.Name() == "help" || cmd.Parent() == nil {
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

	var cmdRegister = &cobra.Command{
		Use:   "register-repo [repo-identifier]",
		Short: "Registra um novo repositório no Controller",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			provider, _ := cmd.Flags().GetString("provider")
			if err := ctrl.RegisterRepository(args[0], provider); err != nil {
				fmt.Printf("❌ Erro: %v\n", err)
				os.Exit(1)
			}
		},
	}
	cmdRegister.Flags().StringP("provider", "p", "azure", "SCM provider")

	var cmdLogout = &cobra.Command{
		Use:   "logout",
		Short: "Remove credenciais salvas",
		Run: func(cmd *cobra.Command, args []string) {
			if err := ctrl.Logout(); err != nil {
				fmt.Printf("❌ Erro: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("✅ Logout realizado!")
		},
	}

	var cmdWhoami = &cobra.Command{
		Use:   "whoami",
		Short: "Mostra informações do usuário logado",
		Run: func(cmd *cobra.Command, args []string) {
			if err := ctrl.Whoami(); err != nil {
				fmt.Printf("❌ Erro: %v\n", err)
				os.Exit(1)
			}
		},
	}

	// Comandos que NÃO dependem do config.Load()
	var cmdInit = &cobra.Command{
		Use:   "init",
		Short: "Configura credenciais do OAuth",
		Run: func(cmd *cobra.Command, args []string) {
			if err := config.Initialize(); err != nil {
				fmt.Printf("❌ Erro: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("✅ Configuração salva!")
		},
	}

	rootCmd.AddCommand(cmdRegister, cmdLogout, cmdWhoami, cmdInit)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
