package cmd

import (
	"math"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// Feature: claude-auth-cli, Property 7: Time remaining calculation
// Validates: Requirements 5.3

func TestPropertyTimeRemainingCalculation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random duration between 1 minute and 24 hours
		totalSeconds := rapid.IntRange(60, 24*60*60).Draw(t, "totalSeconds")

		now := time.Now()
		expiry := now.Add(time.Duration(totalSeconds) * time.Second)

		hours, minutes := formatTimeRemainingFrom(expiry, now)

		// The total displayed minutes should equal floor(total_minutes_remaining)
		displayedTotal := hours*60 + minutes
		expectedTotal := int(math.Floor(float64(totalSeconds) / 60.0))

		if displayedTotal != expectedTotal {
			t.Fatalf("formatTimeRemainingFrom with %d seconds remaining: got %dh %dm (total %d min), expected %d min",
				totalSeconds, hours, minutes, displayedTotal, expectedTotal)
		}

		// Hours and minutes should be non-negative
		if hours < 0 || minutes < 0 {
			t.Fatalf("formatTimeRemainingFrom returned negative values: hours=%d, minutes=%d", hours, minutes)
		}

		// Minutes component should be less than 60
		if minutes >= 60 {
			t.Fatalf("formatTimeRemainingFrom returned minutes >= 60: %d", minutes)
		}
	})
}
