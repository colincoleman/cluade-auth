package cmd

import (
	"strings"
	"testing"

	"github.com/ksgit/claude-auth/internal/config"
	"pgregory.net/rapid"
)

// Feature: claude-auth-cli, Property 6: Exec environment correctness
// Validates: Requirements 4.3, 4.5

func TestPropertyExecEnvironmentCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random non-empty workspace region and workspace ID
		region := rapid.StringMatching(`[a-z]{2}-[a-z]+-[0-9]`).Draw(t, "region")
		workspaceID := rapid.StringMatching(`wrkspc_[a-zA-Z0-9]{5,20}`).Draw(t, "workspaceID")

		// Generate a random non-empty API key string
		apiKey := rapid.StringMatching(`aws-external-anthropic-api-key-[A-Za-z0-9+/=]{10,50}`).Draw(t, "apiKey")

		cfg := &config.Config{
			WorkspaceRegion: region,
			WorkspaceID:     workspaceID,
		}

		env := buildExecEnv(cfg, apiKey)

		// Convert env slice to a map for easier lookup of injected variables.
		// We only care about the LAST occurrence of each key (since buildExecEnv
		// appends, later values override earlier ones in exec semantics).
		envMap := make(map[string]string)
		for _, entry := range env {
			if k, v, ok := strings.Cut(entry, "="); ok {
				envMap[k] = v
			}
		}

		// Assert required variables are present with correct values
		expectedVars := map[string]string{
			"CLAUDE_CODE_USE_ANTHROPIC_AWS": "1",
			"AWS_REGION":                    region,
			"ANTHROPIC_AWS_WORKSPACE_ID":    workspaceID,
			"ANTHROPIC_AWS_API_KEY":         apiKey,
		}

		for key, want := range expectedVars {
			got, ok := envMap[key]
			if !ok {
				t.Fatalf("expected env var %s to be present, but it was not found", key)
			}
			if got != want {
				t.Fatalf("env var %s = %q, want %q", key, got, want)
			}
		}

		// Assert that raw AWS credentials are NOT injected by buildExecEnv.
		// We check that these keys do not appear in the injected portion of the env.
		// Since buildExecEnv appends to os.Environ(), we check the injected slice
		// (the last 4 entries) does not contain these keys.
		injectedCount := 4
		injectedSlice := env[len(env)-injectedCount:]
		forbidden := []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN"}

		for _, entry := range injectedSlice {
			key, _, _ := strings.Cut(entry, "=")
			for _, forbiddenKey := range forbidden {
				if key == forbiddenKey {
					t.Fatalf("injected env must NOT contain %s, but found it", forbiddenKey)
				}
			}
		}
	})
}
