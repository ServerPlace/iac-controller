package cmd

import (
	"fmt"

	"github.com/ServerPlace/iac-controller/internal/admin/config"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Configura credenciais do OAuth",
	Run: func(cmd *cobra.Command, args []string) {
		if err := config.Initialize(); err != nil {
			fmt.Printf("❌ Erro: %v\n", err)
			return
		}
		fmt.Println("✅ Configuração salva!")
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
