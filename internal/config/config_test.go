package config_test

import (
	"os"
	"testing"
	"time"

	"github.com/ksgit/claude-auth/internal/config"
)

func TestDefaultConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	if cfg.Vault == "" {
		t.Error("default vault should not be empty")
	}
	if cfg.SessionDuration <= 0 {
		t.Error("default session duration should be positive")
	}
}

func TestDefaultConfigHasRoleAndRegion(t *testing.T) {
	cfg := config.DefaultConfig()
	if cfg.RoleARN == "" {
		t.Error("default RoleARN should not be empty")
	}
	if cfg.WorkspaceRegion == "" {
		t.Error("default WorkspaceRegion should not be empty")
	}
}

func TestSaveAndLoad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	want := &config.Config{
		OnePasswordAccount: "test@example.com",
		Vault:              "My Vault",
		Item:               "AWS Creds",
		RoleARN:            "arn:aws:iam::123:role/test",
		MFASerial:          "arn:aws:iam::123:mfa/test",
		WorkspaceRegion:    "eu-west-1",
		WorkspaceID:        "ws-abc123",
		SessionDuration:    12,
	}

	if err := config.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.OnePasswordAccount != want.OnePasswordAccount {
		t.Errorf("OnePasswordAccount: got %q, want %q", got.OnePasswordAccount, want.OnePasswordAccount)
	}
	if got.RoleARN != want.RoleARN {
		t.Errorf("RoleARN: got %q, want %q", got.RoleARN, want.RoleARN)
	}
	if got.WorkspaceRegion != want.WorkspaceRegion {
		t.Errorf("WorkspaceRegion: got %q, want %q", got.WorkspaceRegion, want.WorkspaceRegion)
	}
	if got.WorkspaceID != want.WorkspaceID {
		t.Errorf("WorkspaceID: got %q, want %q", got.WorkspaceID, want.WorkspaceID)
	}
}

func TestExists(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if config.Exists() {
		t.Error("Exists should be false before Save")
	}

	cfg := config.DefaultConfig()
	cfg.OnePasswordAccount = "a"
	cfg.WorkspaceID = "ws-x"
	if err := config.Save(&cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if !config.Exists() {
		t.Error("Exists should be true after Save")
	}
}

func TestLoadMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	_, err := config.Load()
	if err == nil {
		t.Error("Load should return an error when config is missing")
	}
}

func TestSaveAndLoadState(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	expiry := time.Now().Add(12 * time.Hour).UTC().Truncate(time.Second)
	want := &config.State{
		AnthropicTokenExpiry: expiry.Format(time.RFC3339),
	}

	if err := config.SaveState(want); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	got, err := config.LoadState()
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if got.AnthropicTokenExpiry != want.AnthropicTokenExpiry {
		t.Errorf("AnthropicTokenExpiry: got %q, want %q", got.AnthropicTokenExpiry, want.AnthropicTokenExpiry)
	}
}

func TestLoadStateMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	state, err := config.LoadState()
	if err != nil {
		t.Fatalf("LoadState on missing file should not error: %v", err)
	}
	if state == nil {
		t.Error("LoadState should return an empty state, not nil")
	}
}

func TestConfigFilePermissions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg := config.DefaultConfig()
	cfg.OnePasswordAccount = "test"
	cfg.WorkspaceID = "ws-test"
	if err := config.Save(&cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	path, _ := config.Path()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("config file permissions: got %o, want 0600", perm)
	}
}
