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

	// Step 1: fetch long-term IAM credentials from 1Password
	fmt.Println("Fetching credentials from 1Password…")
	opClient, err := onepw.New(ctx, cfg.OnePasswordAccount)
	if err != nil {
		return err
	}
	accessKeyID, secretAccessKey, err := opClient.GetCredentials(ctx, cfg.Vault, cfg.Item)
	if err != nil {
		return err
	}

	// Step 2: exchange for short-term STS token (try preferred region, fall back)
	fmt.Printf("Requesting STS session token (region: %s)…\n", cfg.AWSRegion)
	shortTermCreds, err := awscreds.GetSessionToken(ctx, accessKeyID, secretAccessKey, cfg.AWSRegion, cfg.SessionDuration)
	if err != nil {
		fmt.Printf("Primary region failed (%v), trying fallback %s…\n", err, cfg.AWSRegionFallback)
		shortTermCreds, err = awscreds.GetSessionToken(ctx, accessKeyID, secretAccessKey, cfg.AWSRegionFallback, cfg.SessionDuration)
		if err != nil {
			return fmt.Errorf("STS failed on both regions: %w", err)
		}
	}

	// Step 3: write short-term AWS credentials to ~/.aws/credentials
	fmt.Printf("Writing [%s] profile to ~/.aws/credentials…\n", cfg.AWSProfile)
	if err := awscreds.WriteProfile(cfg.AWSProfile, shortTermCreds); err != nil {
		return fmt.Errorf("failed to write credentials: %w", err)
	}

	// Step 4: generate short-lived Anthropic API key token
	fmt.Println("Generating Anthropic API key token…")
	awsCreds, err := shortTermCreds.ToAWSCredentials()
	if err != nil {
		return err
	}
	duration := time.Duration(cfg.SessionDuration) * time.Hour
	anthropicToken, err := tokengen.Generate(ctx, awsCreds, cfg.AWSRegion, duration)
	if err != nil {
		// Non-fatal: AWS creds still work via SigV4
		fmt.Printf("Warning: Anthropic token generation failed: %v\n", err)
		fmt.Println("AWS SigV4 credentials are still active — Claude will use those instead.")
	} else {
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
	}

	// Step 5: save expiry state
	state := &config.State{
		AWSExpiry:            shortTermCreds.Expiry.UTC().Format(time.RFC3339),
		AnthropicTokenExpiry: shortTermCreds.Expiry.UTC().Format(time.RFC3339),
	}
	_ = config.SaveState(state)

	fmt.Printf("\nCredentials valid until: %s\n", shortTermCreds.Expiry.Local().Format("2006-01-02 15:04 MST"))
	fmt.Println("Run 'claude-auth status' to check remaining time.")
	return nil
}

