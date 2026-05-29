package cmd

import (
	"fmt"
	"time"

	"github.com/ksgit/claude-auth/internal/config"
	"github.com/ksgit/claude-auth/internal/tokengen"
	"github.com/spf13/cobra"
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Verify the claude-auth setup is working correctly",
	RunE:  runCheck,
}

func runCheck(_ *cobra.Command, _ []string) error {
	fmt.Println("claude-auth check")
	fmt.Println("─────────────────")

	// 1. Config
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("✗ Config         %v\n", err)
		return err
	}
	path, _ := config.Path()
	fmt.Printf("✓ Config         %s\n", path)
	fmt.Printf("  Workspace:     %s  (region %s)\n", cfg.WorkspaceID, cfg.EffectiveWorkspaceRegion())

	// 2. Token present
	apiKey := readAnthropicAPIKey()
	if apiKey == "" {
		fmt.Println("✗ API key token  not found — run 'claude-auth refresh'")
		return nil
	}
	envPath, _ := config.EnvPath()
	fmt.Printf("✓ API key token  %s\n", envPath)

	// 3. Decode the token locally — region + expiry (no network call)
	info, err := tokengen.Decode(apiKey)
	if err != nil {
		fmt.Printf("✗ Token decode   %v\n", err)
		return nil
	}

	// Region match
	want := cfg.EffectiveWorkspaceRegion()
	if info.Region == want {
		fmt.Printf("✓ Token region   %s (matches workspace)\n", info.Region)
	} else {
		fmt.Printf("✗ Token region   %s — does NOT match workspace region %s\n", info.Region, want)
		fmt.Println("  → Run 'claude-auth refresh' to regenerate for the correct region")
	}

	// Expiry
	if info.Expiry.IsZero() {
		fmt.Println("⚠ Token expiry   could not determine")
	} else {
		remaining := time.Until(info.Expiry)
		switch {
		case remaining <= 0:
			fmt.Println("✗ Token expiry   EXPIRED — run 'claude-auth refresh'")
		case remaining < 30*time.Minute:
			fmt.Printf("⚠ Token expiry   %dm remaining — refresh soon\n", int(remaining.Minutes()))
		default:
			h := int(remaining.Hours())
			m := int(remaining.Minutes()) % 60
			fmt.Printf("✓ Token expiry   %dh %dm remaining\n", h, m)
		}
	}

	fmt.Println("\nNote: if 'claude-auth exec -- claude' returns a 403, your IAM principal")
	fmt.Println("lacks aws-external-anthropic:CreateInference. Attach the")
	fmt.Println("AnthropicInferenceAccess managed policy in AWS Console → IAM.")
	return nil
}
