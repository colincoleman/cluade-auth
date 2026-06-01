package mfa

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// Feature: mfa-rate-limiting, Property 4: Cooldown configuration validation
// **Validates: Requirements 3.4, 3.5, 3.6**

func TestPropertyValidateCooldownMinutes(t *testing.T) {
	t.Run("valid_non_negative_integers_accepted", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			// Generate a non-negative integer and convert to string
			val := rapid.IntRange(0, 100000).Draw(t, "validCooldown")
			input := strconv.Itoa(val)

			result, err := ValidateCooldownMinutes(input)
			if err != nil {
				t.Fatalf("ValidateCooldownMinutes(%q) returned error %v, want nil for valid non-negative integer", input, err)
			}
			if result != val {
				t.Fatalf("ValidateCooldownMinutes(%q) = %d, want %d", input, result, val)
			}
		})
	})

	t.Run("valid_non_negative_integers_with_whitespace_accepted", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			// Generate a non-negative integer with leading/trailing whitespace
			val := rapid.IntRange(0, 100000).Draw(t, "validCooldown")
			leadingSpaces := rapid.IntRange(0, 5).Draw(t, "leadingSpaces")
			trailingSpaces := rapid.IntRange(0, 5).Draw(t, "trailingSpaces")
			input := strings.Repeat(" ", leadingSpaces) + strconv.Itoa(val) + strings.Repeat(" ", trailingSpaces)

			result, err := ValidateCooldownMinutes(input)
			if err != nil {
				t.Fatalf("ValidateCooldownMinutes(%q) returned error %v, want nil for valid non-negative integer with whitespace", input, err)
			}
			if result != val {
				t.Fatalf("ValidateCooldownMinutes(%q) = %d, want %d", input, result, val)
			}
		})
	})

	t.Run("negative_integers_rejected", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			// Generate a negative integer
			val := rapid.IntRange(-100000, -1).Draw(t, "negativeCooldown")
			input := strconv.Itoa(val)

			_, err := ValidateCooldownMinutes(input)
			if err == nil {
				t.Fatalf("ValidateCooldownMinutes(%q) returned nil error, want error for negative integer", input)
			}
		})
	})

	t.Run("decimal_values_rejected", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			// Generate decimal strings like "3.14", "0.5", "-1.2"
			intPart := rapid.IntRange(-100, 100).Draw(t, "intPart")
			fracPart := rapid.IntRange(1, 99).Draw(t, "fracPart") // non-zero fractional part
			input := fmt.Sprintf("%d.%d", intPart, fracPart)

			_, err := ValidateCooldownMinutes(input)
			if err == nil {
				t.Fatalf("ValidateCooldownMinutes(%q) returned nil error, want error for decimal value", input)
			}
		})
	})

	t.Run("non_numeric_text_rejected", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			// Generate strings that contain at least one non-numeric character (after trimming)
			category := rapid.IntRange(0, 3).Draw(t, "category")

			var input string
			switch category {
			case 0:
				// Pure alphabetic text
				input = rapid.StringMatching(`[a-zA-Z]{1,20}`).Draw(t, "alphaText")
			case 1:
				// Mixed alphanumeric that isn't a pure integer
				input = rapid.StringMatching(`[0-9]*[a-zA-Z]+[0-9]*`).Draw(t, "mixedText")
			case 2:
				// Special characters
				input = rapid.StringMatching(`[!@#$%^&*()_+=]{1,10}`).Draw(t, "specialChars")
			case 3:
				// Empty string
				input = ""
			}

			_, err := ValidateCooldownMinutes(input)
			if err == nil {
				t.Fatalf("ValidateCooldownMinutes(%q) returned nil error, want error for non-numeric text", input)
			}
		})
	})

	t.Run("result_is_always_non_negative_on_success", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			// Generate arbitrary strings and verify that if the function succeeds,
			// the result is always non-negative
			input := rapid.String().Draw(t, "arbitraryInput")

			result, err := ValidateCooldownMinutes(input)
			if err == nil && result < 0 {
				t.Fatalf("ValidateCooldownMinutes(%q) returned negative value %d with nil error", input, result)
			}
		})
	})
}

// TestPropertyFormatRemaining verifies that FormatRemaining produces correctly
// formatted output for any duration within the cooldown window range.
//
// Feature: mfa-rate-limiting, Property 3: Cooldown time remaining formatting
// **Validates: Requirements 2.6, 5.1**
//
// For any duration between 0 and the maximum cooldown window, FormatRemaining
// SHALL produce a string in the format "Xh Ym" where X is the truncated hours
// and Y is the truncated remaining minutes, such that X*60 + Y equals the total
// minutes obtained by truncating the duration to whole minutes.
func TestPropertyFormatRemaining(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random duration in seconds (0 to 7 days worth of seconds)
		// This covers well beyond any reasonable cooldown window
		totalSeconds := rapid.IntRange(0, 7*24*60*60).Draw(t, "totalSeconds")
		d := time.Duration(totalSeconds) * time.Second

		result := FormatRemaining(d)

		// Verify format: must match "Xh Ym" pattern
		var hours, minutes int
		n, err := fmt.Sscanf(result, "%dh %dm", &hours, &minutes)
		if err != nil || n != 2 {
			t.Fatalf("FormatRemaining(%v) = %q, does not match format 'Xh Ym': err=%v, matched=%d", d, result, err, n)
		}

		// Verify hours and minutes are non-negative
		if hours < 0 {
			t.Fatalf("FormatRemaining(%v) = %q, hours is negative: %d", d, result, hours)
		}
		if minutes < 0 {
			t.Fatalf("FormatRemaining(%v) = %q, minutes is negative: %d", d, result, minutes)
		}

		// Verify minutes component is less than 60
		if minutes >= 60 {
			t.Fatalf("FormatRemaining(%v) = %q, minutes component %d >= 60", d, result, minutes)
		}

		// Verify arithmetic correctness: X*60 + Y == truncated total minutes
		expectedTotalMinutes := int(d.Minutes())
		actualTotalMinutes := hours*60 + minutes
		if actualTotalMinutes != expectedTotalMinutes {
			t.Fatalf("FormatRemaining(%v) = %q, arithmetic mismatch: %d*60 + %d = %d, expected %d total minutes",
				d, result, hours, minutes, actualTotalMinutes, expectedTotalMinutes)
		}
	})
}
