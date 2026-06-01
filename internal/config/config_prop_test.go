package config

import (
	"os"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// Feature: claude-auth-cli, Property 1: Config serialization round-trip
// **Validates: Requirements 1.4, 1.6**
//
// For any valid Config struct (with non-empty required fields and session duration
// between 1 and 12), saving it via config.Save and then loading it via config.Load
// SHALL produce a struct equal to the original, and the written file SHALL have
// permissions 0600.
func TestProperty1_ConfigSerializationRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	rapid.Check(t, func(rt *rapid.T) {
		// Generate a random Config with non-empty required fields
		cfg := &Config{
			OnePasswordAccount: rapid.StringMatching(`[a-zA-Z0-9@._]{1,50}`).Draw(rt, "onepassword_account"),
			Vault:              rapid.StringMatching(`[a-zA-Z0-9 _-]{1,30}`).Draw(rt, "vault"),
			Item:               rapid.StringMatching(`[a-zA-Z0-9 _-]{1,30}`).Draw(rt, "item"),
			RoleARN:            rapid.StringMatching(`arn:aws:iam::[0-9]{12}:role/[a-zA-Z0-9_-]{1,30}`).Draw(rt, "role_arn"),
			MFASerial:          rapid.OneOf(rapid.Just(""), rapid.StringMatching(`arn:aws:iam::[0-9]{12}:mfa/[a-zA-Z0-9._-]{1,30}`)).Draw(rt, "mfa_serial"),
			WorkspaceRegion:    rapid.StringMatching(`[a-z]{2}-[a-z]+-[0-9]`).Draw(rt, "workspace_region"),
			WorkspaceID:        rapid.StringMatching(`ws-[a-zA-Z0-9]{4,20}`).Draw(rt, "workspace_id"),
			SessionDuration:    rapid.IntRange(1, 12).Draw(rt, "session_duration"),
		}

		// Save the config
		if err := Save(cfg); err != nil {
			rt.Fatalf("Save failed: %v", err)
		}

		// Load the config back
		loaded, err := Load()
		if err != nil {
			rt.Fatalf("Load failed: %v", err)
		}

		// Assert equality of all fields
		if loaded.OnePasswordAccount != cfg.OnePasswordAccount {
			rt.Errorf("OnePasswordAccount: got %q, want %q", loaded.OnePasswordAccount, cfg.OnePasswordAccount)
		}
		if loaded.Vault != cfg.Vault {
			rt.Errorf("Vault: got %q, want %q", loaded.Vault, cfg.Vault)
		}
		if loaded.Item != cfg.Item {
			rt.Errorf("Item: got %q, want %q", loaded.Item, cfg.Item)
		}
		if loaded.RoleARN != cfg.RoleARN {
			rt.Errorf("RoleARN: got %q, want %q", loaded.RoleARN, cfg.RoleARN)
		}
		if loaded.MFASerial != cfg.MFASerial {
			rt.Errorf("MFASerial: got %q, want %q", loaded.MFASerial, cfg.MFASerial)
		}
		if loaded.WorkspaceRegion != cfg.WorkspaceRegion {
			rt.Errorf("WorkspaceRegion: got %q, want %q", loaded.WorkspaceRegion, cfg.WorkspaceRegion)
		}
		if loaded.WorkspaceID != cfg.WorkspaceID {
			rt.Errorf("WorkspaceID: got %q, want %q", loaded.WorkspaceID, cfg.WorkspaceID)
		}
		if loaded.SessionDuration != cfg.SessionDuration {
			rt.Errorf("SessionDuration: got %d, want %d", loaded.SessionDuration, cfg.SessionDuration)
		}

		// Verify file permissions are 0600
		path, err := Path()
		if err != nil {
			rt.Fatalf("Path failed: %v", err)
		}
		info, err := os.Stat(path)
		if err != nil {
			rt.Fatalf("Stat failed: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0600 {
			rt.Errorf("file permissions: got %o, want 0600", perm)
		}
	})
}

// Feature: claude-auth-cli, Property 5: State serialization round-trip
// **Validates: Requirements 3.8**
//
// For any valid RFC3339 timestamp string, saving a State with that timestamp
// via config.SaveState and then loading it via config.LoadState SHALL produce
// a State with the same AnthropicTokenExpiry value.
func TestProperty5_StateSerializationRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	rapid.Check(t, func(rt *rapid.T) {
		// Generate a random time.Time and format as RFC3339
		// Use a reasonable range: year 2000 to year 2100
		year := rapid.IntRange(2000, 2100).Draw(rt, "year")
		month := rapid.IntRange(1, 12).Draw(rt, "month")
		day := rapid.IntRange(1, 28).Draw(rt, "day") // use 28 to avoid invalid dates
		hour := rapid.IntRange(0, 23).Draw(rt, "hour")
		minute := rapid.IntRange(0, 59).Draw(rt, "minute")
		second := rapid.IntRange(0, 59).Draw(rt, "second")

		ts := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC)
		rfc3339Str := ts.Format(time.RFC3339)

		// Save state with the generated timestamp
		state := &State{
			AnthropicTokenExpiry: rfc3339Str,
		}
		err := SaveState(state)
		if err != nil {
			rt.Fatalf("SaveState failed: %v", err)
		}

		// Load state back
		loaded, err := LoadState()
		if err != nil {
			rt.Fatalf("LoadState failed: %v", err)
		}

		// Assert round-trip equality
		if loaded.AnthropicTokenExpiry != rfc3339Str {
			rt.Fatalf("round-trip mismatch: saved %q, loaded %q", rfc3339Str, loaded.AnthropicTokenExpiry)
		}
	})
}

// Feature: mfa-rate-limiting, Property 5: Config default value for missing cooldown field
// **Validates: Requirements 3.1**
//
// For any valid config JSON that does not contain the `mfa_cooldown_minutes` field,
// loading the config SHALL yield a cooldown value of 60 minutes via GetMFACooldownMinutes().
func TestPropertyConfigDefaultCooldown(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	rapid.Check(t, func(rt *rapid.T) {
		// Generate a random Config with non-empty required fields but NO MFACooldownMinutes
		cfg := &Config{
			OnePasswordAccount: rapid.StringMatching(`[a-zA-Z0-9@._]{1,50}`).Draw(rt, "onepassword_account"),
			Vault:              rapid.StringMatching(`[a-zA-Z0-9 _-]{1,30}`).Draw(rt, "vault"),
			Item:               rapid.StringMatching(`[a-zA-Z0-9 _-]{1,30}`).Draw(rt, "item"),
			RoleARN:            rapid.StringMatching(`arn:aws:iam::[0-9]{12}:role/[a-zA-Z0-9_-]{1,30}`).Draw(rt, "role_arn"),
			MFASerial:          rapid.OneOf(rapid.Just(""), rapid.StringMatching(`arn:aws:iam::[0-9]{12}:mfa/[a-zA-Z0-9._-]{1,30}`)).Draw(rt, "mfa_serial"),
			WorkspaceRegion:    rapid.StringMatching(`[a-z]{2}-[a-z]+-[0-9]`).Draw(rt, "workspace_region"),
			WorkspaceID:        rapid.StringMatching(`ws-[a-zA-Z0-9]{4,20}`).Draw(rt, "workspace_id"),
			SessionDuration:    rapid.IntRange(1, 12).Draw(rt, "session_duration"),
			MFACooldownMinutes: nil, // explicitly nil — field omitted from JSON
		}

		// Save the config (MFACooldownMinutes is nil so omitempty will exclude it)
		if err := Save(cfg); err != nil {
			rt.Fatalf("Save failed: %v", err)
		}

		// Verify the written JSON does not contain the mfa_cooldown_minutes key
		path, err := Path()
		if err != nil {
			rt.Fatalf("Path failed: %v", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			rt.Fatalf("ReadFile failed: %v", err)
		}
		for _, line := range splitLines(data) {
			if containsKey(line, "mfa_cooldown_minutes") {
				rt.Fatalf("config JSON unexpectedly contains mfa_cooldown_minutes: %s", string(data))
			}
		}

		// Load the config back
		loaded, err := Load()
		if err != nil {
			rt.Fatalf("Load failed: %v", err)
		}

		// The MFACooldownMinutes pointer should be nil after loading
		if loaded.MFACooldownMinutes != nil {
			rt.Errorf("MFACooldownMinutes should be nil when field is absent, got %d", *loaded.MFACooldownMinutes)
		}

		// GetMFACooldownMinutes() must return the default of 60
		if got := loaded.GetMFACooldownMinutes(); got != 60 {
			rt.Errorf("GetMFACooldownMinutes() = %d, want 60", got)
		}
	})
}

// splitLines splits byte data into lines for inspection.
func splitLines(data []byte) []string {
	var lines []string
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, string(data[start:i]))
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, string(data[start:]))
	}
	return lines
}

// containsKey checks if a line contains a JSON key.
func containsKey(line, key string) bool {
	return len(line) > 0 && contains(line, `"`+key+`"`)
}

// contains is a simple substring check.
func contains(s, substr string) bool {
	return len(substr) <= len(s) && indexSubstring(s, substr) >= 0
}

// indexSubstring returns the index of substr in s, or -1 if not found.
func indexSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
