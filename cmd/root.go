package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:          "claude-auth",
	Short:        "Manage AWS and Claude Platform credentials",
	Long:         "claude-auth stores long-term AWS IAM credentials in 1Password and refreshes short-term credentials for use with Claude Platform on AWS.",
	SilenceUsage: true,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(storeCmd)
	rootCmd.AddCommand(refreshCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(execCmd)
}
