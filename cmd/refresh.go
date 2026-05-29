package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ksgit/claude-auth/internal/config"
	"github.com/ksgit/claude-auth/internal/tokengen"
	"github.com/spf13/cobra"
)

var refreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Refresh short-term AWS credentials and Anthropic API key",
	RunE:  runRefresh,
}

func runRefresh(_ *cobra.Command, _ []string) error {
	cfg, err := loadConfigOrSetup()
	if err != nil {
		return err
	}

	ctx := context.Background()

	// Step 1: assume the configured role (1Password creds + MFA). These temp
	// creds hold CreateInference; used only to sign the token — never written to disk.
	shortTermCreds, err := assumeConfiguredRole(ctx, cfg)
	if err != nil {
		return err
	}

	// Step 2: generate the short-lived Anthropic API key token
	fmt.Println("Generating Anthropic API key token…")
	awsCreds, err := shortTermCreds.ToAWSCredentials()
	if err != nil {
		return err
	}
	duration := time.Duration(cfg.SessionDuration) * time.Hour
	anthropicToken, err := tokengen.Generate(ctx, awsCreds, cfg.WorkspaceRegion, duration)
	if err != nil {
		return fmt.Errorf("failed to generate Anthropic API key token: %w", err)
	}

	envPath, err := config.EnvPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(envPath), 0700); err != nil {
		return err
	}
	content := fmt.Sprintf("ANTHROPIC_AWS_API_KEY=%s\n", anthropicToken)
	if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write anthropic.env: %w", err)
	}
	fmt.Printf("Wrote ANTHROPIC_AWS_API_KEY to %s\n", envPath)

	// Step 4: save expiry state
	state := &config.State{
		AnthropicTokenExpiry: shortTermCreds.Expiry.UTC().Format(time.RFC3339),
	}
	_ = config.SaveState(state)

	fmt.Printf("\nToken valid until: %s\n", shortTermCreds.Expiry.Local().Format("2006-01-02 15:04 MST"))

	// Warn if the role capped the session below what we asked for.
	requested := time.Duration(cfg.SessionDuration) * time.Hour
	if actual := time.Until(shortTermCreds.Expiry); actual < requested-5*time.Minute {
		fmt.Printf("\nNote: you requested %dh but the role granted only ~%.0fh.\n", cfg.SessionDuration, actual.Hours()+0.5)
		fmt.Println("This is the role's MaxSessionDuration. Raise it (up to 12h) with:")
		fmt.Printf("  claude-auth aws-exec -- aws iam update-role --role-name %s --max-session-duration %d\n",
			roleName(cfg.RoleARN), cfg.SessionDuration*3600)
	}

	fmt.Println("\nRun 'claude-auth status' to check remaining time.")
	return nil
}

// roleName extracts the role name from a role ARN (…:role/NAME).
func roleName(arn string) string {
	if i := strings.LastIndex(arn, "/"); i >= 0 {
		return arn[i+1:]
	}
	return arn
}

