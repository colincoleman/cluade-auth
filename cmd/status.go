package cmd

import (
	"fmt"
	"time"

	"github.com/ksgit/claude-auth/internal/config"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show time remaining on current credentials",
	RunE:  runStatus,
}

func runStatus(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	state, err := config.LoadState()
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	fmt.Printf("Profile: [%s]\n\n", cfg.AWSProfile)

	printExpiry("AWS credentials", state.AWSExpiry)
	printExpiry("Anthropic token", state.AnthropicTokenExpiry)

	return nil
}

func printExpiry(label, isoTime string) {
	if isoTime == "" {
		fmt.Printf("  %-22s  not set — run 'claude-auth refresh'\n", label)
		return
	}
	t, err := time.Parse(time.RFC3339, isoTime)
	if err != nil {
		fmt.Printf("  %-22s  unknown\n", label)
		return
	}

	remaining := time.Until(t)
	if remaining <= 0 {
		fmt.Printf("  %-22s  EXPIRED (%s)\n", label, t.Local().Format("2006-01-02 15:04 MST"))
		return
	}

	h := int(remaining.Hours())
	m := int(remaining.Minutes()) % 60
	fmt.Printf("  %-22s  %dh %dm remaining (expires %s)\n",
		label, h, m, t.Local().Format("15:04 MST"))
}
