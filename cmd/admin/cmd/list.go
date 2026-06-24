package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list-repos",
	Short: "Lista todos os repositórios registrados",
	Run: func(cmd *cobra.Command, args []string) {
		if err := ctrl.ListRepositories(); err != nil {
			fmt.Printf("❌ Erro: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
