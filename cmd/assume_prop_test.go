package cmd

import (
	"testing"

	"pgregory.net/rapid"
)

// Feature: claude-auth-cli, Property 8: MFA code format validation
// Validates: Requirements 10.4

func TestPropertyMFACodeFormatValidation_ValidCodes(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random string that IS exactly 6 ASCII decimal digits
		code := rapid.StringMatching(`[0-9]{6}`).Draw(t, "validMFACode")

		err := validateMFACode(code)
		if err != nil {
			t.Fatalf("validateMFACode(%q) returned error %v, want nil", code, err)
		}
	})
}

func TestPropertyMFACodeFormatValidation_InvalidCodes(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random string that is NOT exactly 6 ASCII decimal digits.
		// Strategy: pick from several categories of invalid inputs.
		category := rapid.IntRange(0, 4).Draw(t, "category")

		var code string
		switch category {
		case 0:
			// Wrong length: 0-5 digits
			length := rapid.IntRange(0, 5).Draw(t, "shortLen")
			code = rapid.StringMatching(`[0-9]{` + itoa(length) + `}`).Draw(t, "shortDigits")
		case 1:
			// Wrong length: 7+ digits
			length := rapid.IntRange(7, 20).Draw(t, "longLen")
			code = rapid.StringMatching(`[0-9]{` + itoa(length) + `}`).Draw(t, "longDigits")
		case 2:
			// Exactly 6 characters but contains non-digit characters
			code = rapid.StringMatching(`[a-zA-Z!@#$%^&*() ]{6}`).Draw(t, "nonDigit6")
		case 3:
			// Mixed: some digits and some non-digits, length 6
			code = rapid.StringMatching(`[0-9a-zA-Z!@#]{6}`).Draw(t, "mixed6")
			// Ensure it's not all digits (re-draw if it happens to be)
			if isAllDigits(code) {
				code = rapid.StringMatching(`[a-z]{1}[0-9]{5}`).Draw(t, "forcedNonDigit6")
			}
		case 4:
			// Arbitrary string of random length (likely invalid)
			code = rapid.String().Draw(t, "arbitrary")
			// Ensure it's not accidentally a valid 6-digit code
			if len(code) == 6 && isAllDigits(code) {
				code = code + "x"
			}
		}

		err := validateMFACode(code)
		if err == nil {
			t.Fatalf("validateMFACode(%q) returned nil, want error for invalid MFA code", code)
		}
	})
}

// itoa converts a small int to its string representation for regex building.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// isAllDigits returns true if s consists entirely of ASCII decimal digits.
func isAllDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
