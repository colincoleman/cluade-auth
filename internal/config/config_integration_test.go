package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ksgit/claude-auth/internal/config"
	"github.com/ksgit/claude-auth/internal/mfa"
)

// TestStateFilePermissions verifies that SaveState creates the state file
// with 0600 permissions (requirement 1.5).
func TestStateFilePermissions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	state := &config.State{
		AnthropicTokenExpiry: time.Now().UTC().Format(time.RFC3339),
		LastMFASuccess:       time.Now().UTC().Format(time.RFC3339),
	}

	if err := config.SaveState(state); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	path, err := config.StatePath()
	if err != nil {
		t.Fatalf("StatePath: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat state file: %v", err)
	}

	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("state file permissions: got %o, want 0600", perm)
	}
}

// TestStateDirectoryPermissions verifies that SaveState creates the config
// directory with 0700 permissions (requirement 1.5).
func TestStateDirectoryPermissions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	state := &config.State{
		AnthropicTokenExpiry: time.Now().UTC().Format(time.RFC3339),
	}

	if err := config.SaveState(state); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	dir, err := config.Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat directory: %v", err)
	}

	if perm := info.Mode().Perm(); perm != 0700 {
		t.Errorf("config directory permissions: got %o, want 0700", perm)
	}
}

// TestClearRemovesStateAndNextRefreshRequiresMFA verifies that after the state
// file is removed (as `clear` does), the MFA tracker finds no timestamp and
// requires MFA (requirements 7.2, 7.3).
func TestClearRemovesStateAndNextRefreshRequiresMFA(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Simulate a state with a recent MFA timestamp (within cooldown)
	now := time.Now().UTC()
	state := &config.State{
		AnthropicTokenExpiry: now.Add(12 * time.Hour).Format(time.RFC3339),
		LastMFASuccess:       now.Format(time.RFC3339),
	}
	if err := config.SaveState(state); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	// Verify MFA would be skipped (cooldown active)
	tracker := mfa.NewTrackerWithClock(func() time.Time { return now.Add(10 * time.Minute) })
	loaded, err := config.LoadState()
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	lastMFA := mfa.ParseMFATimestamp(loaded.LastMFASuccess)
	skip, _ := tracker.ShouldSkipMFA(lastMFA, 60)
	if !skip {
		t.Fatal("expected MFA to be skipped before clear (cooldown active)")
	}

	// Simulate `clear` by removing the state file
	statePath, err := config.StatePath()
	if err != nil {
		t.Fatalf("StatePath: %v", err)
	}
	if err := os.Remove(statePath); err != nil {
		t.Fatalf("Remove state file: %v", err)
	}

	// After clear, LoadState returns empty state (no MFA timestamp)
	clearedState, err := config.LoadState()
	if err != nil {
		t.Fatalf("LoadState after clear: %v", err)
	}
	if clearedState.LastMFASuccess != "" {
		t.Errorf("expected empty LastMFASuccess after clear, got %q", clearedState.LastMFASuccess)
	}

	// MFA tracker should NOT skip MFA (no timestamp → zero time → requires MFA)
	lastMFA = mfa.ParseMFATimestamp(clearedState.LastMFASuccess)
	skip, _ = tracker.ShouldSkipMFA(lastMFA, 60)
	if skip {
		t.Error("expected MFA to be required after clear (no timestamp in state)")
	}
}

// TestFullCooldownFlow verifies the complete MFA cooldown lifecycle:
// 1. After MFA → timestamp recorded → within cooldown (skip)
// 2. After cooldown expires → MFA required again
// (requirements 1.5, 7.2, 7.3)
func TestFullCooldownFlow(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Phase 1: Initial state — no MFA timestamp, MFA is required
	state, err := config.LoadState()
	if err != nil {
		t.Fatalf("LoadState (initial): %v", err)
	}
	lastMFA := mfa.ParseMFATimestamp(state.LastMFASuccess)
	if !lastMFA.IsZero() {
		t.Fatal("expected zero lastMFA initially")
	}

	mfaTime := time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)
	tracker := mfa.NewTrackerWithClock(func() time.Time { return mfaTime })

	skip, _ := tracker.ShouldSkipMFA(lastMFA, 60)
	if skip {
		t.Error("expected MFA required on first refresh (no timestamp)")
	}

	// Phase 2: Simulate successful MFA — record timestamp
	timestamp := tracker.RecordMFA()
	state.LastMFASuccess = timestamp
	if err := config.SaveState(state); err != nil {
		t.Fatalf("SaveState after MFA: %v", err)
	}

	// Phase 3: Refresh within cooldown — MFA should be skipped
	withinCooldown := mfaTime.Add(30 * time.Minute)
	tracker2 := mfa.NewTrackerWithClock(func() time.Time { return withinCooldown })

	loaded, err := config.LoadState()
	if err != nil {
		t.Fatalf("LoadState (within cooldown): %v", err)
	}
	lastMFA = mfa.ParseMFATimestamp(loaded.LastMFASuccess)
	skip, remaining := tracker2.ShouldSkipMFA(lastMFA, 60)
	if !skip {
		t.Error("expected MFA to be skipped within cooldown window")
	}
	if remaining != 30*time.Minute {
		t.Errorf("expected 30m remaining, got %v", remaining)
	}

	// Phase 4: Refresh after cooldown expires — MFA required again
	afterCooldown := mfaTime.Add(61 * time.Minute)
	tracker3 := mfa.NewTrackerWithClock(func() time.Time { return afterCooldown })

	skip, _ = tracker3.ShouldSkipMFA(lastMFA, 60)
	if skip {
		t.Error("expected MFA required after cooldown expires")
	}
}

// TestStateFileCreatedFromScratch verifies that SaveState creates both the
// directory and file from scratch with correct permissions when neither exists.
func TestStateFileCreatedFromScratch(t *testing.T) {
	tmpDir := t.TempDir()
	// Use a subdirectory that doesn't exist yet to verify MkdirAll behavior
	t.Setenv("HOME", filepath.Join(tmpDir, "nonexistent"))

	state := &config.State{
		AnthropicTokenExpiry: time.Now().UTC().Format(time.RFC3339),
		LastMFASuccess:       time.Now().UTC().Format(time.RFC3339),
	}

	if err := config.SaveState(state); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	// Verify directory permissions
	dir, err := config.Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat dir: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0700 {
		t.Errorf("directory permissions: got %o, want 0700", perm)
	}

	// Verify file permissions
	path, err := config.StatePath()
	if err != nil {
		t.Fatalf("StatePath: %v", err)
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat file: %v", err)
	}
	if perm := fileInfo.Mode().Perm(); perm != 0600 {
		t.Errorf("state file permissions: got %o, want 0600", perm)
	}

	// Verify the state can be loaded back
	loaded, err := config.LoadState()
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if loaded.LastMFASuccess != state.LastMFASuccess {
		t.Errorf("LastMFASuccess: got %q, want %q", loaded.LastMFASuccess, state.LastMFASuccess)
	}
}
