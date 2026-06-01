package cmd

import (
	"fmt"
	"os"

	"github.com/ksgit/claude-auth/internal/config"
	"github.com/spf13/cobra"
)

var clearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Remove the stored Anthropic token",
	Long: `Delete the locally stored ANTHROPIC_AWS_API_KEY token and its expiry state.

You don't need this just to use your personal Claude account — the token is only
ever injected by 'claude-auth exec', so running 'claude' directly already uses
your personal account. Use 'clear' for hygiene or to force a fresh 'refresh'.`,
	RunE: runClear,
}

func runClear(_ *cobra.Command, _ []string) error {
	paths := []func() (string, error){config.EnvPath, config.StatePath}
	removed := 0
	for _, pathFn := range paths {
		p, err := pathFn()
		if err != nil {
			return err
		}
		if err := os.Remove(p); err == nil {
			removed++
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove %s: %w", p, err)
		}
	}

	if removed == 0 {
		fmt.Println("Nothing to clear — no stored token or MFA state.")
	} else {
		fmt.Println("Cleared the stored Anthropic token and MFA state.")
	}
	fmt.Println("Plain `claude` uses your personal account; run `claude-auth refresh` for an AWS token again.")
	return nil
}
