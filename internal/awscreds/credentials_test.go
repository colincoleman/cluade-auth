package awscreds_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ksgit/claude-auth/internal/awscreds"
)

func TestWriteAndReadProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	creds := &awscreds.SessionCredentials{
		AccessKeyID:     "ASIATEST123",
		SecretAccessKey: "secretkey456",
		SessionToken:    "tokenvalue789",
		Expiry:          time.Now().Add(12 * time.Hour),
	}

	if err := awscreds.WriteProfile("myprofile", creds); err != nil {
		t.Fatalf("WriteProfile: %v", err)
	}

	credPath := filepath.Join(home, ".aws", "credentials")
	data, err := os.ReadFile(credPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)

	for _, want := range []string{
		"[myprofile]",
		"aws_access_key_id     = ASIATEST123",
		"aws_secret_access_key = secretkey456",
		"aws_session_token     = tokenvalue789",
	} {
		if !containsNormalised(content, want) {
			t.Errorf("credentials file missing %q\ncontent:\n%s", want, content)
		}
	}
}

func TestWriteProfileCreatesDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	creds := &awscreds.SessionCredentials{
		AccessKeyID:     "AK",
		SecretAccessKey: "SK",
		SessionToken:    "ST",
		Expiry:          time.Now().Add(time.Hour),
	}
	if err := awscreds.WriteProfile("test", creds); err != nil {
		t.Fatalf("WriteProfile: %v", err)
	}

	if _, err := os.Stat(filepath.Join(home, ".aws")); err != nil {
		t.Errorf(".aws dir should have been created: %v", err)
	}
}

func TestWriteProfileUpdatesExisting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	first := &awscreds.SessionCredentials{
		AccessKeyID: "FIRST", SecretAccessKey: "SK1", SessionToken: "ST1",
		Expiry: time.Now().Add(time.Hour),
	}
	second := &awscreds.SessionCredentials{
		AccessKeyID: "SECOND", SecretAccessKey: "SK2", SessionToken: "ST2",
		Expiry: time.Now().Add(time.Hour),
	}

	if err := awscreds.WriteProfile("claude", first); err != nil {
		t.Fatalf("first WriteProfile: %v", err)
	}
	if err := awscreds.WriteProfile("claude", second); err != nil {
		t.Fatalf("second WriteProfile: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(home, ".aws", "credentials"))
	content := string(data)

	if containsNormalised(content, "FIRST") {
		t.Error("old access key should have been overwritten")
	}
	if !containsNormalised(content, "SECOND") {
		t.Error("new access key should be present")
	}
}

func TestWriteProfilePreservesOtherProfiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Write an existing profile that should not be touched
	credPath := filepath.Join(home, ".aws", "credentials")
	if err := os.MkdirAll(filepath.Dir(credPath), 0700); err != nil {
		t.Fatal(err)
	}
	existing := "[default]\naws_access_key_id = DEFAULT_KEY\naws_secret_access_key = DEFAULT_SECRET\n"
	if err := os.WriteFile(credPath, []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}

	creds := &awscreds.SessionCredentials{
		AccessKeyID: "CLAUDE_KEY", SecretAccessKey: "CLAUDE_SECRET", SessionToken: "TOK",
		Expiry: time.Now().Add(time.Hour),
	}
	if err := awscreds.WriteProfile("claude", creds); err != nil {
		t.Fatalf("WriteProfile: %v", err)
	}

	data, _ := os.ReadFile(credPath)
	content := string(data)

	if !containsNormalised(content, "DEFAULT_KEY") {
		t.Error("[default] profile should be preserved")
	}
	if !containsNormalised(content, "CLAUDE_KEY") {
		t.Error("[claude] profile should be present")
	}
}

func TestToAWSCredentials(t *testing.T) {
	expiry := time.Now().Add(time.Hour)
	sc := &awscreds.SessionCredentials{
		AccessKeyID:     "AK",
		SecretAccessKey: "SK",
		SessionToken:    "ST",
		Expiry:          expiry,
	}
	creds, err := sc.ToAWSCredentials()
	if err != nil {
		t.Fatalf("ToAWSCredentials: %v", err)
	}
	if creds.AccessKeyID != "AK" {
		t.Errorf("AccessKeyID: got %q, want AK", creds.AccessKeyID)
	}
	if !creds.CanExpire {
		t.Error("CanExpire should be true")
	}
}

// containsNormalised checks whether s contains want after collapsing runs of
// spaces (ini.v1 may add or remove spaces around the = sign).
func containsNormalised(s, want string) bool {
	norm := func(in string) string {
		out := make([]byte, 0, len(in))
		prev := byte(0)
		for i := 0; i < len(in); i++ {
			b := in[i]
			if b == ' ' || b == '\t' {
				if prev != ' ' {
					out = append(out, ' ')
					prev = ' '
				}
			} else {
				out = append(out, b)
				prev = b
			}
		}
		return string(out)
	}
	return contains(norm(s), norm(want))
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsHelper(s, sub))
}

func containsHelper(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
