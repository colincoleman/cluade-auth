package mfa

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Tracker manages MFA cooldown state: reading/writing timestamps and
// evaluating whether MFA should be skipped.
type Tracker struct {
	clock func() time.Time
}

// NewTracker creates a Tracker using the real system clock.
func NewTracker() *Tracker {
	return &Tracker{clock: func() time.Time { return time.Now().UTC() }}
}

// NewTrackerWithClock creates a Tracker with an injectable clock for testing.
func NewTrackerWithClock(clock func() time.Time) *Tracker {
	return &Tracker{clock: clock}
}

// ShouldSkipMFA determines whether MFA can be skipped based on the last
// successful MFA time and the configured cooldown duration.
// Returns (skip bool, remaining time.Duration).
// If lastMFA is zero or cooldown is 0, returns (false, 0).
func (t *Tracker) ShouldSkipMFA(lastMFA time.Time, cooldownMinutes int) (bool, time.Duration) {
	if lastMFA.IsZero() || cooldownMinutes <= 0 {
		return false, 0
	}

	cooldown := time.Duration(cooldownMinutes) * time.Minute
	elapsed := t.clock().Sub(lastMFA)

	if elapsed < cooldown {
		return true, cooldown - elapsed
	}

	return false, 0
}

// RecordMFA returns the current UTC time formatted as RFC3339, suitable
// for persisting to state.
func (t *Tracker) RecordMFA() string {
	return t.clock().UTC().Format(time.RFC3339)
}

// ParseMFATimestamp parses an RFC3339 string into a time.Time.
// Returns zero time if the string is empty or invalid.
func ParseMFATimestamp(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// FormatRemaining formats a duration as "Xh Ym" with truncation to whole minutes.
func FormatRemaining(d time.Duration) string {
	if d <= 0 {
		return "0h 0m"
	}

	totalMinutes := int(d.Minutes())
	hours := totalMinutes / 60
	minutes := totalMinutes % 60

	return fmt.Sprintf("%dh %dm", hours, minutes)
}

// ValidateCooldownMinutes validates a string input for the cooldown config field.
// Returns the parsed integer value or an error if invalid (non-integer, negative).
func ValidateCooldownMinutes(input string) (int, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return 0, fmt.Errorf("cooldown must be a whole number")
	}

	val, err := strconv.Atoi(input)
	if err != nil {
		return 0, fmt.Errorf("cooldown must be a whole number")
	}

	if val < 0 {
		return 0, fmt.Errorf("cooldown must be zero or a positive integer")
	}

	return val, nil
}
