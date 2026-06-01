package cmd

import (
	"fmt"
	"time"

	"github.com/ksgit/claude-auth/internal/config"
	"github.com/ksgit/claude-auth/internal/mfa"
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

	fmt.Printf("Workspace region: %s\n\n", cfg.WorkspaceRegion)

	printExpiry("Anthropic token", state.AnthropicTokenExpiry)

	printMFACooldownStatus(cfg, state)

	return nil
}

// printMFACooldownStatus displays the MFA cooldown status based on config and state.
func printMFACooldownStatus(cfg *config.Config, state *config.State) {
	// Requirement 5.6: If MFA serial is empty, omit MFA cooldown info entirely
	if cfg.MFASerial == "" {
		return
	}

	fmt.Println()

	// Requirement 5.4: If cooldown is 0, display disabled message
	cooldownMinutes := cfg.GetMFACooldownMinutes()
	if cooldownMinutes == 0 {
		fmt.Println("  MFA rate-limiting is disabled")
		return
	}

	// Requirement 5.3/5.5: If no timestamp or invalid timestamp
	lastMFA := mfa.ParseMFATimestamp(state.LastMFASuccess)
	if lastMFA.IsZero() {
		fmt.Println("  MFA has not been used yet")
		return
	}

	// Evaluate cooldown status
	tracker := mfa.NewTracker()
	skip, remaining := tracker.ShouldSkipMFA(lastMFA, cooldownMinutes)

	if skip {
		// Requirement 5.1: Within cooldown, display time remaining
		fmt.Printf("  MFA cooldown          %s remaining\n", mfa.FormatRemaining(remaining))
	} else {
		// Requirement 5.2: Cooldown expired
		fmt.Println("  Next refresh will require MFA")
	}
}

// formatTimeRemaining returns the hours and minutes remaining until the given
// expiry time relative to now, truncated to whole minutes (not rounded).
func formatTimeRemaining(expiry time.Time) (hours int, minutes int) {
	return formatTimeRemainingFrom(expiry, time.Now())
}

// formatTimeRemainingFrom returns the hours and minutes remaining until expiry
// relative to the given reference time, truncated to whole minutes.
func formatTimeRemainingFrom(expiry time.Time, now time.Time) (hours int, minutes int) {
	remaining := expiry.Sub(now)
	totalMinutes := int(remaining.Minutes())
	return totalMinutes / 60, totalMinutes % 60
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

	h, m := formatTimeRemaining(t)
	fmt.Printf("  %-22s  %dh %dm remaining (expires %s)\n",
		label, h, m, t.Local().Format("15:04 MST"))
}
