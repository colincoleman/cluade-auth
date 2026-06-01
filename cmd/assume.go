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

// fetchOPCredentials fetches the long-term IAM credentials from 1Password.
// Shared by both MFA and non-MFA role assumption paths.
func fetchOPCredentials(ctx context.Context, cfg *config.Config) (*onepw.Credentials, error) {
	fmt.Println("Fetching credentials from 1Password…")
	opClient, err := onepw.New(ctx, cfg.OnePasswordAccount)
	if err != nil {
		return nil, err
	}
	return opClient.GetCredentials(ctx, cfg.Vault, cfg.Item)
}

// assumeRoleWithoutMFA fetches long-term IAM credentials from 1Password and
// assumes the configured role without MFA parameters (no SerialNumber or TokenCode).
// Used when the MFA cooldown is active or when no MFA serial is configured.
func assumeRoleWithoutMFA(ctx context.Context, cfg *config.Config) (*awscreds.SessionCredentials, error) {
	creds, err := fetchOPCredentials(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return assumeRoleWithoutMFAUsingCreds(ctx, cfg, creds)
}

// assumeRoleWithoutMFAUsingCreds assumes the role without MFA using pre-fetched credentials.
func assumeRoleWithoutMFAUsingCreds(ctx context.Context, cfg *config.Config, creds *onepw.Credentials) (*awscreds.SessionCredentials, error) {
	fmt.Printf("Assuming role %s (region: %s) without MFA…\n", cfg.RoleARN, cfg.WorkspaceRegion)
	return awscreds.AssumeRole(ctx, creds.AccessKeyID, creds.SecretAccessKey,
		cfg.RoleARN, "", "", cfg.WorkspaceRegion, cfg.SessionDuration)
}

// assumeConfiguredRole fetches the long-term IAM credentials (and MFA TOTP) from
// 1Password and assumes the configured role with MFA, returning short-term credentials.
// This is the MFA path: it fetches the TOTP from 1Password or prompts the user.
// Shared by 'refresh' (to sign the token) and 'aws-exec' (to run AWS commands).
func assumeConfiguredRole(ctx context.Context, cfg *config.Config) (*awscreds.SessionCredentials, error) {
	creds, err := fetchOPCredentials(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return assumeRoleWithMFAUsingCreds(ctx, cfg, creds)
}

// assumeRoleWithMFAUsingCreds assumes the role with MFA using pre-fetched credentials.
// If the credentials include a TOTP code, it's used directly; otherwise the user is prompted.
func assumeRoleWithMFAUsingCreds(ctx context.Context, cfg *config.Config, creds *onepw.Credentials) (*awscreds.SessionCredentials, error) {
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
