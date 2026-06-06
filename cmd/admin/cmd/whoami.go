package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Mostra informações do usuário logado",
	Run: func(cmd *cobra.Command, args []string) {
		if err := ctrl.Whoami(); err != nil {
			fmt.Printf("❌ Erro: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(whoamiCmd)
}
