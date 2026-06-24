package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update-repo [repo-id]",
	Short: "Atualiza metadados de um repositório registrado",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		keyVersion, _ := cmd.Flags().GetInt("key-version")
		if err := ctrl.UpdateRepository(args[0], keyVersion); err != nil {
			fmt.Printf("❌ Erro: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	updateCmd.Flags().IntP("key-version", "k", 2, "Nova versão da chave HMAC")
	rootCmd.AddCommand(updateCmd)
}
