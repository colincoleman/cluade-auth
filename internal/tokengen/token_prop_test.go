package tokengen_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/ksgit/claude-auth/internal/tokengen"
	"pgregory.net/rapid"
)

// genAWSRegion generates a random valid AWS region string.
func genAWSRegion() *rapid.Generator[string] {
	regions := []string{
		"us-east-1", "us-east-2", "us-west-1", "us-west-2",
		"eu-west-1", "eu-west-2", "eu-west-3", "eu-central-1", "eu-north-1",
		"ap-southeast-1", "ap-southeast-2", "ap-northeast-1", "ap-northeast-2",
		"ap-south-1", "sa-east-1", "ca-central-1", "me-south-1",
	}
	return rapid.SampledFrom(regions)
}

// genAccessKeyID generates a random valid AWS access key ID (starts with AKIA, 20 chars total).
func genAccessKeyID() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		suffix := rapid.StringMatching(`[A-Z0-9]{16}`).Draw(t, "accessKeySuffix")
		return "AKIA" + suffix
	})
}

// genSecretAccessKey generates a random 40-character secret access key.
func genSecretAccessKey() *rapid.Generator[string] {
	return rapid.StringMatching(`[A-Za-z0-9/+=]{40}`)
}

// genSessionToken generates a random session token (non-empty).
func genSessionToken() *rapid.Generator[string] {
	return rapid.StringMatching(`[A-Za-z0-9/+=]{20,100}`)
}

// genDuration generates a random duration between 1 second and 12 hours.
func genDuration() *rapid.Generator[time.Duration] {
	return rapid.Custom(func(t *rapid.T) time.Duration {
		secs := rapid.IntRange(1, 43200).Draw(t, "durationSecs")
		return time.Duration(secs) * time.Second
	})
}

// Feature: claude-auth-cli, Property 4: Token structural validity
// **Validates: Requirements 8.1, 8.2, 8.3, 8.4**
func TestPropertyTokenStructuralValidity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random valid AWS credentials and region
		accessKeyID := genAccessKeyID().Draw(t, "accessKeyID")
		secretAccessKey := genSecretAccessKey().Draw(t, "secretAccessKey")
		sessionToken := genSessionToken().Draw(t, "sessionToken")
		region := genAWSRegion().Draw(t, "region")
		duration := genDuration().Draw(t, "duration")

		creds := aws.Credentials{
			AccessKeyID:     accessKeyID,
			SecretAccessKey:  secretAccessKey,
			SessionToken:    sessionToken,
			CanExpire:       true,
			Expires:         time.Now().Add(time.Hour),
		}

		token, err := tokengen.Generate(context.Background(), creds, region, duration)
		if err != nil {
			t.Fatalf("Generate failed: %v", err)
		}

		// Assert token starts with the expected prefix
		const prefix = "aws-external-anthropic-api-key-"
		if !strings.HasPrefix(token, prefix) {
			t.Fatalf("token does not start with prefix %q, got: %s", prefix, token[:min(len(token), 60)])
		}

		// Assert payload after prefix is valid base64
		encoded := strings.TrimPrefix(token, prefix)
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			t.Fatalf("payload after prefix is not valid base64: %v", err)
		}

		payload := string(decoded)

		// Assert decoded payload contains Action=CallWithBearerToken (Req 8.1)
		if !strings.Contains(payload, "Action=CallWithBearerToken") {
			t.Error("decoded payload missing Action=CallWithBearerToken")
		}

		// Assert decoded payload contains X-Amz-Algorithm= (Req 8.2)
		if !strings.Contains(payload, "X-Amz-Algorithm=") {
			t.Error("decoded payload missing X-Amz-Algorithm=")
		}

		// Assert decoded payload contains X-Amz-Credential= with region in correct position (Req 8.2)
		if !strings.Contains(payload, "X-Amz-Credential=") {
			t.Error("decoded payload missing X-Amz-Credential=")
		} else {
			// Extract X-Amz-Credential value and verify region is in the third slash-delimited segment
			credIdx := strings.Index(payload, "X-Amz-Credential=")
			credValue := payload[credIdx+len("X-Amz-Credential="):]
			// Value ends at next & or end of string
			if ampIdx := strings.Index(credValue, "&"); ampIdx >= 0 {
				credValue = credValue[:ampIdx]
			}
			// URL-decode the credential value (slashes are encoded as %2F in query params)
			credDecoded, err := url.QueryUnescape(credValue)
			if err != nil {
				t.Fatalf("failed to URL-decode X-Amz-Credential value: %v", err)
			}
			parts := strings.Split(credDecoded, "/")
			if len(parts) < 3 {
				t.Errorf("X-Amz-Credential has fewer than 3 slash-delimited segments: %q", credDecoded)
			} else if parts[2] != region {
				t.Errorf("X-Amz-Credential region mismatch: got %q in position 3, want %q", parts[2], region)
			}
		}

		// Assert decoded payload contains X-Amz-Expires= matching duration in seconds (Req 8.3)
		expectedExpires := fmt.Sprintf("X-Amz-Expires=%d", int(duration.Seconds()))
		if !strings.Contains(payload, expectedExpires) {
			t.Errorf("decoded payload missing %q", expectedExpires)
		}

		// Assert decoded payload ends with &Version=1 (Req 8.4)
		if !strings.HasSuffix(payload, "&Version=1") {
			t.Errorf("decoded payload does not end with &Version=1, ends with: ...%s", payload[max(0, len(payload)-20):])
		}
	})
}
