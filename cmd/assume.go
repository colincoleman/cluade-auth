package cmd

import (
	"context"
	"fmt"

	"github.com/ksgit/claude-auth/internal/awscreds"
	"github.com/ksgit/claude-auth/internal/config"
	"github.com/ksgit/claude-auth/internal/onepw"
)

// assumeConfiguredRole fetches the long-term IAM credentials (and MFA TOTP) from
// 1Password and assumes the configured role, returning short-term credentials.
// Shared by 'refresh' (to sign the token) and 'aws-exec' (to run AWS commands).
func assumeConfiguredRole(ctx context.Context, cfg *config.Config) (*awscreds.SessionCredentials, error) {
	fmt.Println("Fetching credentials from 1Password…")
	opClient, err := onepw.New(ctx, cfg.OnePasswordAccount)
	if err != nil {
		return nil, err
	}
	creds, err := opClient.GetCredentials(ctx, cfg.Vault, cfg.Item)
	if err != nil {
		return nil, err
	}

	tokenCode := creds.TOTP
	if cfg.MFASerial != "" && tokenCode == "" {
		tokenCode = prompt(fmt.Sprintf("MFA code for %s", cfg.MFASerial), "")
		if tokenCode == "" {
			return nil, fmt.Errorf("an MFA code is required to assume %s", cfg.RoleARN)
		}
	}

	fmt.Printf("Assuming role %s (region: %s)…\n", cfg.RoleARN, cfg.WorkspaceRegion)
	return awscreds.AssumeRole(ctx, creds.AccessKeyID, creds.SecretAccessKey,
		cfg.RoleARN, cfg.MFASerial, tokenCode, cfg.WorkspaceRegion, cfg.SessionDuration)
}
