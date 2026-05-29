package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ksgit/claude-auth/internal/awscreds"
	"github.com/ksgit/claude-auth/internal/config"
	"github.com/ksgit/claude-auth/internal/onepw"
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
	region := cfg.WorkspaceRegion

	// Step 1: fetch long-term IAM credentials + MFA TOTP from 1Password (one prompt)
	fmt.Println("Fetching credentials from 1Password…")
	opClient, err := onepw.New(ctx, cfg.OnePasswordAccount)
	if err != nil {
		return err
	}
	creds, err := opClient.GetCredentials(ctx, cfg.Vault, cfg.Item)
	if err != nil {
		return err
	}

	// MFA: use the TOTP from 1Password if present, otherwise prompt for the code.
	tokenCode := creds.TOTP
	if cfg.MFASerial != "" && tokenCode == "" {
		tokenCode = prompt(fmt.Sprintf("MFA code for %s", cfg.MFASerial), "")
		if tokenCode == "" {
			return fmt.Errorf("an MFA code is required to assume %s", cfg.RoleARN)
		}
	}

	// Step 2: assume the role (with MFA) to get temp creds that hold
	// CreateInference. These are used only to sign the token — never written to disk.
	fmt.Printf("Assuming role %s (region: %s)…\n", cfg.RoleARN, region)
	shortTermCreds, err := awscreds.AssumeRole(ctx, creds.AccessKeyID, creds.SecretAccessKey,
		cfg.RoleARN, cfg.MFASerial, tokenCode, region, cfg.SessionDuration)
	if err != nil {
		return err
	}

	// Step 3: generate the short-lived Anthropic API key token
	fmt.Println("Generating Anthropic API key token…")
	awsCreds, err := shortTermCreds.ToAWSCredentials()
	if err != nil {
		return err
	}
	duration := time.Duration(cfg.SessionDuration) * time.Hour
	anthropicToken, err := tokengen.Generate(ctx, awsCreds, region, duration)
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
	fmt.Println("Run 'claude-auth status' to check remaining time.")
	return nil
}

