package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var registerCmd = &cobra.Command{
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

func init() {
	registerCmd.Flags().StringP("provider", "p", "azure", "SCM provider")
	rootCmd.AddCommand(registerCmd)
}
