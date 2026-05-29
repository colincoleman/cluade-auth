package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/ksgit/claude-auth/internal/config"
	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Interactive first-run configuration wizard",
	RunE:  runSetup,
}

func runSetup(_ *cobra.Command, _ []string) error {
	fmt.Println("claude-auth setup")
	fmt.Println("─────────────────")
	fmt.Println("Press Enter to accept the default shown in [brackets].\n")

	cfg := config.DefaultConfig()

	cfg.OnePasswordAccount = prompt("1Password account name (shown in app title bar)", "")
	if cfg.OnePasswordAccount == "" {
		return fmt.Errorf("1Password account name is required")
	}

	cfg.Vault = promptWithDefault("1Password vault", cfg.Vault)
	cfg.Item = promptWithDefault("1Password item name", cfg.Item)
	cfg.AWSProfile = promptWithDefault("AWS credentials profile", cfg.AWSProfile)
	cfg.AWSRegion = promptWithDefault("Preferred AWS region", cfg.AWSRegion)
	cfg.AWSRegionFallback = promptWithDefault("Fallback AWS region", cfg.AWSRegionFallback)
	cfg.WorkspaceID = prompt("Anthropic workspace ID (Claude Platform on AWS → Workspaces)", "")
	if cfg.WorkspaceID == "" {
		return fmt.Errorf("workspace ID is required")
	}

	if err := config.Save(&cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	path, _ := config.Path()
	fmt.Printf("\nConfig saved to %s\n", path)
	fmt.Println("\nNext steps:")
	fmt.Println("  1. In 1Password: Settings → Developer → enable \"Integrate with 1Password CLI\"")
	fmt.Println("  2. Run: claude-auth store")
	fmt.Println("  3. Run: claude-auth refresh")
	fmt.Printf("\nAdd to ~/.zshrc:\n")
	fmt.Println("  [ -f ~/.config/claude-auth/anthropic.env ] && source ~/.config/claude-auth/anthropic.env")
	fmt.Printf("  export ANTHROPIC_AWS_WORKSPACE_ID=%s\n", cfg.WorkspaceID)
	fmt.Println("  function claude() { AWS_PROFILE=" + cfg.AWSProfile + " command claude \"$@\"; }")

	return nil
}

func prompt(label, defaultVal string) string {
	reader := bufio.NewReader(os.Stdin)
	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", label, defaultVal)
	} else {
		fmt.Printf("%s: ", label)
	}
	line, _ := reader.ReadString('\n')
	val := strings.TrimSpace(line)
	if val == "" {
		return defaultVal
	}
	return val
}

func promptWithDefault(label, defaultVal string) string {
	return prompt(label, defaultVal)
}
