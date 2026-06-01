package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ksgit/claude-auth/internal/awscreds"
	"github.com/ksgit/claude-auth/internal/config"
	"github.com/ksgit/claude-auth/internal/mfa"
	"github.com/ksgit/claude-auth/internal/tokengen"
	"github.com/spf13/cobra"
)

var forceMFA bool

// errSkipRefresh is a sentinel error indicating the refresh should be skipped
// because the existing token is still valid and MFA cooldown is active.
var errSkipRefresh = fmt.Errorf("skip refresh")

var refreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Refresh short-term AWS credentials and Anthropic API key",
	RunE:  runRefresh,
}

func init() {
	refreshCmd.Flags().BoolVar(&forceMFA, "force-mfa", false, "Force MFA authentication even within the cooldown window")
}

func runRefresh(_ *cobra.Command, _ []string) error {
	cfg, err := loadConfigOrSetup()
	if err != nil {
		return err
	}

	ctx := context.Background()

	// Step 1: assume the configured role with MFA cooldown evaluation.
	shortTermCreds, err := assumeWithCooldown(ctx, cfg)
	if err == errSkipRefresh {
		return nil
	}
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

	// Step 4: save expiry state (preserve existing MFA timestamp)
	existingState, _ := config.LoadState()
	state := &config.State{
		AnthropicTokenExpiry: shortTermCreds.Expiry.UTC().Format(time.RFC3339),
		LastMFASuccess:       existingState.LastMFASuccess,
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

// assumeWithCooldown evaluates the MFA cooldown and --force-mfa flag to decide
// whether to perform MFA or skip it. Handles AccessDenied fallback when MFA-less
// assumption fails, and records MFA timestamps on successful MFA authentication.
func assumeWithCooldown(ctx context.Context, cfg *config.Config) (*awscreds.SessionCredentials, error) {
	// If --force-mfa is set, validate and force MFA path
	if forceMFA {
		if cfg.MFASerial == "" {
			return nil, fmt.Errorf("--force-mfa requires MFA to be configured")
		}

		// Check if cooldown is active to display the forcing message
		state, _ := config.LoadState()
		lastMFA := mfa.ParseMFATimestamp(state.LastMFASuccess)
		tracker := mfa.NewTracker()
		skip, _ := tracker.ShouldSkipMFA(lastMFA, cfg.GetMFACooldownMinutes())
		if skip {
			fmt.Println("Forcing MFA authentication despite active cooldown window.")
		}

		creds, err := assumeConfiguredRole(ctx, cfg)
		if err != nil {
			return nil, err
		}
		// Record MFA timestamp on successful forced MFA
		recordMFATimestamp()
		return creds, nil
	}

	// If no MFA serial configured, assume without MFA (no timestamp recording)
	if cfg.MFASerial == "" {
		return assumeRoleWithoutMFA(ctx, cfg)
	}

	// Evaluate MFA cooldown
	state, _ := config.LoadState()
	lastMFA := mfa.ParseMFATimestamp(state.LastMFASuccess)
	tracker := mfa.NewTracker()
	skip, remaining := tracker.ShouldSkipMFA(lastMFA, cfg.GetMFACooldownMinutes())

	if skip {
		// Check if the existing token is still valid — if so, skip refresh entirely.
		// The role requires MFA for AssumeRole, so we can't assume without it.
		// Rather than prompting for MFA within the cooldown window, we just reuse
		// the existing token (which was obtained with MFA and is still valid).
		if tokenStillValid(state) {
			h, m := formatTimeRemainingFrom(mustParseTime(state.AnthropicTokenExpiry), time.Now())
			fmt.Printf("MFA cooldown active (%s remaining) — existing token still valid (%dh %dm left). Skipping refresh.\n",
				mfa.FormatRemaining(remaining), h, m)
			return nil, errSkipRefresh
		}

		// Token expired but MFA cooldown still active — must re-authenticate with MFA
		fmt.Printf("Token expired but MFA cooldown still active (%s remaining) — MFA required for new token.\n",
			mfa.FormatRemaining(remaining))
	}

	// Cooldown expired or no previous MFA — proceed with MFA
	creds, err := assumeConfiguredRole(ctx, cfg)
	if err != nil {
		return nil, err
	}
	// Record MFA timestamp on successful MFA authentication
	recordMFATimestamp()
	return creds, nil
}

// tokenStillValid checks whether the existing Anthropic token has not yet expired.
func tokenStillValid(state *config.State) bool {
	if state.AnthropicTokenExpiry == "" {
		return false
	}
	expiry, err := time.Parse(time.RFC3339, state.AnthropicTokenExpiry)
	if err != nil {
		return false
	}
	return time.Now().Before(expiry)
}

// mustParseTime parses an RFC3339 string, returning zero time on failure.
func mustParseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

// isAccessDeniedError checks whether an error is an AWS AccessDenied error.
func isAccessDeniedError(err error) bool {
	return strings.Contains(err.Error(), "AccessDenied")
}

// recordMFATimestamp records the current time as the last successful MFA
// authentication and persists it to the state file. If the state file write
// fails, a warning is logged to stderr and execution continues.
func recordMFATimestamp() {
	tracker := mfa.NewTracker()
	timestamp := tracker.RecordMFA()

	state, _ := config.LoadState()
	state.LastMFASuccess = timestamp

	if err := config.SaveState(state); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save MFA timestamp to state file: %v\n", err)
	}
}

