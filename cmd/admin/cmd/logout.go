package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var logoutCmd = &cobra.Command{
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

func init() {
	rootCmd.AddCommand(logoutCmd)
}
