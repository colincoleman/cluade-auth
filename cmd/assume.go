package cmd

import (
	"context"
	"fmt"
	"regexp"

	"github.com/ksgit/claude-auth/internal/awscreds"
	"github.com/ksgit/claude-auth/internal/config"
	"github.com/ksgit/claude-auth/internal/onepw"
)

// mfaCodeRegex matches exactly 6 ASCII decimal digits.
var mfaCodeRegex = regexp.MustCompile(`^\d{6}$`)

// validateMFACode checks that the given code is exactly 6 ASCII decimal digits.
func validateMFACode(code string) error {
	if !mfaCodeRegex.MatchString(code) {
		return fmt.Errorf("MFA code must be exactly 6 digits")
	}
	return nil
}

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
		if err := validateMFACode(tokenCode); err != nil {
			return nil, err
		}
	}

	fmt.Printf("Assuming role %s (region: %s)…\n", cfg.RoleARN, cfg.WorkspaceRegion)
	return awscreds.AssumeRole(ctx, creds.AccessKeyID, creds.SecretAccessKey,
		cfg.RoleARN, cfg.MFASerial, tokenCode, cfg.WorkspaceRegion, cfg.SessionDuration)
}
