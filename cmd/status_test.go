package cmd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ksgit/claude-auth/internal/config"
)

// captureStdout captures stdout output from a function call.
func captureStdout(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestPrintMFACooldownStatus_EmptyMFASerial(t *testing.T) {
	cfg := &config.Config{MFASerial: ""}
	state := &config.State{}

	output := captureStdout(func() {
		printMFACooldownStatus(cfg, state)
	})

	if output != "" {
		t.Errorf("expected no output for empty MFA serial, got %q", output)
	}
}

func TestPrintMFACooldownStatus_Disabled(t *testing.T) {
	zero := 0
	cfg := &config.Config{
		MFASerial:          "arn:aws:iam::123456789012:mfa/user",
		MFACooldownMinutes: &zero,
	}
	state := &config.State{}

	output := captureStdout(func() {
		printMFACooldownStatus(cfg, state)
	})

	if !strings.Contains(output, "MFA rate-limiting is disabled") {
		t.Errorf("expected disabled message, got %q", output)
	}
}

func TestPrintMFACooldownStatus_NoTimestamp(t *testing.T) {
	cfg := &config.Config{
		MFASerial: "arn:aws:iam::123456789012:mfa/user",
	}
	state := &config.State{LastMFASuccess: ""}

	output := captureStdout(func() {
		printMFACooldownStatus(cfg, state)
	})

	if !strings.Contains(output, "MFA has not been used yet") {
		t.Errorf("expected 'not been used yet' message, got %q", output)
	}
}

func TestPrintMFACooldownStatus_InvalidTimestamp(t *testing.T) {
	cfg := &config.Config{
		MFASerial: "arn:aws:iam::123456789012:mfa/user",
	}
	state := &config.State{LastMFASuccess: "not-a-valid-timestamp"}

	output := captureStdout(func() {
		printMFACooldownStatus(cfg, state)
	})

	if !strings.Contains(output, "MFA has not been used yet") {
		t.Errorf("expected 'not been used yet' message for invalid timestamp, got %q", output)
	}
}

func TestPrintMFACooldownStatus_WithinCooldown(t *testing.T) {
	cfg := &config.Config{
		MFASerial: "arn:aws:iam::123456789012:mfa/user",
	}
	// Set timestamp to 10 minutes ago (within default 60 min cooldown)
	recentTime := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339)
	state := &config.State{LastMFASuccess: recentTime}

	output := captureStdout(func() {
		printMFACooldownStatus(cfg, state)
	})

	if !strings.Contains(output, "remaining") {
		t.Errorf("expected 'remaining' in output for active cooldown, got %q", output)
	}
	if !strings.Contains(output, "MFA cooldown") {
		t.Errorf("expected 'MFA cooldown' label in output, got %q", output)
	}
}

func TestPrintMFACooldownStatus_CooldownExpired(t *testing.T) {
	cfg := &config.Config{
		MFASerial: "arn:aws:iam::123456789012:mfa/user",
	}
	// Set timestamp to 2 hours ago (beyond default 60 min cooldown)
	oldTime := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	state := &config.State{LastMFASuccess: oldTime}

	output := captureStdout(func() {
		printMFACooldownStatus(cfg, state)
	})

	if !strings.Contains(output, "Next refresh will require MFA") {
		t.Errorf("expected 'Next refresh will require MFA' message, got %q", output)
	}
}
