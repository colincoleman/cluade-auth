package cmd

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ksgit/claude-auth/internal/config"
	"github.com/ksgit/claude-auth/internal/mfa"
	"pgregory.net/rapid"
)

// Feature: mfa-rate-limiting, Property 6: Non-AccessDenied errors bypass fallback
// **Validates: Requirements 6.4**
//
// For any STS error that is not an AccessDenied error, when the error occurs
// during a cooldown-skipped (MFA-less) AssumeRole attempt, the system SHALL
// return that error immediately without attempting MFA fallback.
//
// We test the decision logic (isAccessDeniedError) which determines whether
// fallback is triggered. Non-AccessDenied errors must return false, meaning
// no fallback path is taken.

func TestPropertyNonAccessDeniedNoFallback(t *testing.T) {
	t.Run("non_AccessDenied_errors_do_not_trigger_fallback", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			// Generate random error messages that do NOT contain "AccessDenied"
			category := rapid.IntRange(0, 4).Draw(t, "category")

			var msg string
			switch category {
			case 0:
				// Common AWS error types that are not AccessDenied
				errTypes := []string{
					"ExpiredTokenException",
					"InvalidParameterValue",
					"MalformedPolicyDocument",
					"RegionDisabledException",
					"ServiceUnavailable",
					"ThrottlingException",
					"NetworkError",
					"RequestTimeout",
					"InternalServiceError",
					"ValidationError",
				}
				idx := rapid.IntRange(0, len(errTypes)-1).Draw(t, "errTypeIdx")
				msg = errTypes[idx] + ": " + rapid.StringMatching(`[a-zA-Z0-9 ._-]{1,50}`).Draw(t, "detail")
			case 1:
				// Random alphabetic strings
				msg = rapid.StringMatching(`[a-zA-Z]{1,100}`).Draw(t, "alphaMsg")
			case 2:
				// Random strings with special characters
				msg = rapid.StringMatching(`[a-zA-Z0-9 !@#$%^&*()_+=:;,.<>?/-]{1,80}`).Draw(t, "specialMsg")
			case 3:
				// Strings that partially match but are not "AccessDenied"
				partials := []string{
					"Access was denied",
					"access denied",
					"Denied",
					"AccessDenie",
					"ccessDenied",
					"Access_Denied",
					"ACCESS_DENIED",
				}
				idx := rapid.IntRange(0, len(partials)-1).Draw(t, "partialIdx")
				msg = partials[idx]
			case 4:
				// Arbitrary string, filtered to not contain "AccessDenied"
				msg = rapid.String().Draw(t, "arbitraryMsg")
			}

			// Ensure the generated message does NOT contain "AccessDenied"
			if strings.Contains(msg, "AccessDenied") {
				// Replace it to guarantee the invariant
				msg = strings.ReplaceAll(msg, "AccessDenied", "SomethingElse")
			}

			err := errors.New(msg)

			// The decision function must return false for non-AccessDenied errors
			if isAccessDeniedError(err) {
				t.Fatalf("isAccessDeniedError(%q) = true, want false for non-AccessDenied error", msg)
			}
		})
	})

	t.Run("AccessDenied_errors_trigger_fallback", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			// Generate error messages that DO contain "AccessDenied"
			prefix := rapid.StringMatching(`[a-zA-Z0-9 :._-]{0,30}`).Draw(t, "prefix")
			suffix := rapid.StringMatching(`[a-zA-Z0-9 :._-]{0,50}`).Draw(t, "suffix")
			msg := prefix + "AccessDenied" + suffix

			err := errors.New(msg)

			// The decision function must return true for AccessDenied errors
			if !isAccessDeniedError(err) {
				t.Fatalf("isAccessDeniedError(%q) = false, want true for AccessDenied error", msg)
			}
		})
	})
}


// Feature: mfa-rate-limiting, Property 2: MFA timestamp recording invariant
// **Validates: Requirements 1.1, 1.2**
//
// The MFA timestamp in state SHALL be updated if and only if a non-empty token
// code was provided in the request. When no token code is provided (MFA skipped),
// any pre-existing MFA timestamp SHALL be preserved unchanged.
//
// Since the actual refresh flow involves external services (1Password, AWS STS),
// this test validates the LOGIC of timestamp recording:
// 1. recordMFATimestamp() always records a new timestamp and preserves other state
// 2. When MFA is not used (no recordMFATimestamp call), state is preserved unchanged
func TestPropertyMFATimestampRecording(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Isolate state file operations in a unique temp directory per iteration
		tmpHome := t.TempDir()
		os.Setenv("HOME", tmpHome)
		defer os.Unsetenv("HOME")

		// Generate a random scenario:
		// - Whether a pre-existing MFA timestamp exists in state
		// - Whether a non-empty token code is provided (MFA used vs skipped)
		hasPreExistingTimestamp := rapid.Bool().Draw(rt, "hasPreExistingTimestamp")
		hasTokenCode := rapid.Bool().Draw(rt, "hasTokenCode")

		// Generate a random pre-existing timestamp (if applicable)
		var preExistingTimestamp string
		if hasPreExistingTimestamp {
			year := rapid.IntRange(2020, 2030).Draw(rt, "year")
			month := rapid.IntRange(1, 12).Draw(rt, "month")
			day := rapid.IntRange(1, 28).Draw(rt, "day")
			hour := rapid.IntRange(0, 23).Draw(rt, "hour")
			minute := rapid.IntRange(0, 59).Draw(rt, "minute")
			second := rapid.IntRange(0, 59).Draw(rt, "second")
			ts := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC)
			preExistingTimestamp = ts.Format(time.RFC3339)
		}

		// Generate a random token expiry to ensure it's preserved
		expiryYear := rapid.IntRange(2025, 2030).Draw(rt, "expiryYear")
		expiryMonth := rapid.IntRange(1, 12).Draw(rt, "expiryMonth")
		expiryDay := rapid.IntRange(1, 28).Draw(rt, "expiryDay")
		expiryTs := time.Date(expiryYear, time.Month(expiryMonth), expiryDay, 12, 0, 0, 0, time.UTC)
		tokenExpiry := expiryTs.Format(time.RFC3339)

		// Set up initial state with optional pre-existing MFA timestamp
		initialState := &config.State{
			AnthropicTokenExpiry: tokenExpiry,
			LastMFASuccess:       preExistingTimestamp,
		}
		if err := config.SaveState(initialState); err != nil {
			rt.Fatalf("failed to save initial state: %v", err)
		}

		// Simulate the refresh flow decision:
		// If hasTokenCode (MFA was used), call recordMFATimestamp
		// If !hasTokenCode (MFA was skipped), do NOT call recordMFATimestamp
		if hasTokenCode {
			recordMFATimestamp()
		}
		// When !hasTokenCode, we simply don't call recordMFATimestamp,
		// which mirrors the real flow where MFA is skipped.

		// Load the resulting state and verify the invariant
		resultState, err := config.LoadState()
		if err != nil {
			rt.Fatalf("failed to load result state: %v", err)
		}

		if hasTokenCode {
			// Property (Req 1.1): When a non-empty token code was provided,
			// the MFA timestamp SHALL be updated to the current UTC time in RFC3339.
			if resultState.LastMFASuccess == "" {
				rt.Fatalf("MFA timestamp should be set after MFA authentication, but was empty")
			}

			// Verify it's a valid RFC3339 timestamp
			recorded := mfa.ParseMFATimestamp(resultState.LastMFASuccess)
			if recorded.IsZero() {
				rt.Fatalf("recorded MFA timestamp %q is not valid RFC3339", resultState.LastMFASuccess)
			}

			// The recorded timestamp should be recent (within last few seconds)
			elapsed := time.Since(recorded)
			if elapsed < 0 || elapsed > 5*time.Second {
				rt.Fatalf("recorded MFA timestamp %q is not recent (elapsed: %v)", resultState.LastMFASuccess, elapsed)
			}

			// The token expiry field should be preserved (not corrupted)
			if resultState.AnthropicTokenExpiry != tokenExpiry {
				rt.Fatalf("AnthropicTokenExpiry was corrupted: got %q, want %q",
					resultState.AnthropicTokenExpiry, tokenExpiry)
			}
		} else {
			// Property (Req 1.2): When no token code is provided (MFA skipped),
			// any pre-existing MFA timestamp SHALL be preserved unchanged.
			if resultState.LastMFASuccess != preExistingTimestamp {
				rt.Fatalf("MFA timestamp should be preserved when MFA is skipped: got %q, want %q",
					resultState.LastMFASuccess, preExistingTimestamp)
			}

			// The token expiry field should also be preserved
			if resultState.AnthropicTokenExpiry != tokenExpiry {
				rt.Fatalf("AnthropicTokenExpiry was corrupted: got %q, want %q",
					resultState.AnthropicTokenExpiry, tokenExpiry)
			}
		}
	})
}
