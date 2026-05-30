package cmd

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// Feature: claude-auth-cli, Property 2: Prompt returns default on empty input
// Validates: Requirements 1.2

func TestPropertyPromptReturnsDefaultOnEmptyInput(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random non-empty default string (printable ASCII, no newlines)
		defaultVal := rapid.StringMatching(`[a-zA-Z0-9 _\-./]{1,50}`).Draw(t, "defaultVal")

		// Generate empty or whitespace-only input (simulates user pressing Enter without typing)
		whitespaceChars := []string{"", " ", "  ", "\t", " \t ", "   "}
		input := rapid.SampledFrom(whitespaceChars).Draw(t, "input")

		// The prompt function reads from stdin via the scanner.
		// Simulate by providing the input followed by a newline.
		resetScanner(input + "\n")

		got := prompt("TestLabel", defaultVal)

		if got != defaultVal {
			t.Fatalf("prompt(%q, %q) with input %q = %q, want %q",
				"TestLabel", defaultVal, input, got, defaultVal)
		}
	})
}

// TestPropertyPromptReturnsDefaultOnVariousWhitespace tests with generated whitespace patterns.
func TestPropertyPromptReturnsDefaultOnVariousWhitespace(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random non-empty default string
		defaultVal := rapid.StringMatching(`[a-zA-Z0-9_\-]{1,30}`).Draw(t, "defaultVal")

		// Generate whitespace-only input of random length (spaces and tabs)
		wsLen := rapid.IntRange(0, 10).Draw(t, "wsLen")
		var sb strings.Builder
		for i := 0; i < wsLen; i++ {
			if rapid.Bool().Draw(t, "isTab") {
				sb.WriteByte('\t')
			} else {
				sb.WriteByte(' ')
			}
		}
		input := sb.String()

		resetScanner(input + "\n")

		got := prompt("TestLabel", defaultVal)

		if got != defaultVal {
			t.Fatalf("prompt(%q, %q) with whitespace input (len=%d) = %q, want %q",
				"TestLabel", defaultVal, wsLen, got, defaultVal)
		}
	})
}
