package tokengen_test

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/ksgit/claude-auth/internal/tokengen"
)

const (
	tokenPrefix = "aws-external-anthropic-api-key-"
	tokenVersion = "&Version=1"
	serviceHost  = "aws-external-anthropic.amazonaws.com"
)

func fakeCredentials() aws.Credentials {
	return aws.Credentials{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		SessionToken:    "AQoXnyc4lcK4w//////////",
		CanExpire:       true,
		Expires:         time.Now().Add(time.Hour),
	}
}

func TestDecodeRoundTrip(t *testing.T) {
	token, err := tokengen.Generate(context.Background(), fakeCredentials(), "eu-west-1", 6*time.Hour)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	info, err := tokengen.Decode(token)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if info.Region != "eu-west-1" {
		t.Errorf("Region: got %q, want eu-west-1", info.Region)
	}
	if info.Expiry.IsZero() {
		t.Fatal("Expiry should not be zero")
	}
	// Expiry should be ~6h from now (allow generous skew)
	remaining := time.Until(info.Expiry)
	if remaining < 5*time.Hour || remaining > 7*time.Hour {
		t.Errorf("Expiry ~6h expected, got %v remaining", remaining)
	}
}

func TestDecodeRejectsNonToken(t *testing.T) {
	if _, err := tokengen.Decode("sk-ant-not-a-valid-token"); err == nil {
		t.Error("Decode should reject a string without the token prefix")
	}
}

func TestGeneratePrefix(t *testing.T) {
	token, err := tokengen.Generate(context.Background(), fakeCredentials(), "eu-north-1", time.Hour)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.HasPrefix(token, tokenPrefix) {
		t.Errorf("token should start with %q, got: %s", tokenPrefix, token[:min(len(token), 60)])
	}
}

func TestGenerateBase64Payload(t *testing.T) {
	token, err := tokengen.Generate(context.Background(), fakeCredentials(), "eu-north-1", time.Hour)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	encoded := strings.TrimPrefix(token, tokenPrefix)
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("payload is not valid base64: %v", err)
	}

	payload := string(decoded)
	if !strings.HasSuffix(payload, tokenVersion) {
		t.Errorf("decoded payload should end with %q, got: ...%s", tokenVersion, payload[max(0, len(payload)-20):])
	}
	if !strings.HasPrefix(payload, serviceHost) {
		t.Errorf("decoded payload should start with %q, got: %s...", serviceHost, payload[:min(len(payload), 50)])
	}
}

func TestGenerateContainsSigV4Params(t *testing.T) {
	token, err := tokengen.Generate(context.Background(), fakeCredentials(), "eu-north-1", time.Hour)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	encoded := strings.TrimPrefix(token, tokenPrefix)
	decoded, _ := base64.StdEncoding.DecodeString(encoded)
	payload := string(decoded)

	for _, param := range []string{
		"X-Amz-Algorithm=",
		"X-Amz-Credential=",
		"X-Amz-Date=",
		"X-Amz-Expires=",
		"X-Amz-Signature=",
		"Action=CallWithBearerToken",
	} {
		if !strings.Contains(payload, param) {
			t.Errorf("payload missing SigV4 param %q", param)
		}
	}
}

func TestGenerateDifferentRegions(t *testing.T) {
	creds := fakeCredentials()
	t1, err1 := tokengen.Generate(context.Background(), creds, "eu-north-1", time.Hour)
	t2, err2 := tokengen.Generate(context.Background(), creds, "eu-west-1", time.Hour)

	if err1 != nil || err2 != nil {
		t.Fatalf("Generate errors: %v, %v", err1, err2)
	}
	if t1 == t2 {
		t.Error("tokens for different regions should differ")
	}

	// Verify region appears in the decoded payload
	for region, token := range map[string]string{"eu-north-1": t1, "eu-west-1": t2} {
		encoded := strings.TrimPrefix(token, tokenPrefix)
		decoded, _ := base64.StdEncoding.DecodeString(encoded)
		if !strings.Contains(string(decoded), region) {
			t.Errorf("region %q not found in decoded payload", region)
		}
	}
}

func TestGenerateExpiryInPayload(t *testing.T) {
	token, err := tokengen.Generate(context.Background(), fakeCredentials(), "eu-north-1", 6*time.Hour)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	encoded := strings.TrimPrefix(token, tokenPrefix)
	decoded, _ := base64.StdEncoding.DecodeString(encoded)

	// 6 hours = 21600 seconds
	if !strings.Contains(string(decoded), "X-Amz-Expires=21600") {
		t.Errorf("X-Amz-Expires=21600 not found in payload:\n%s", decoded)
	}
}

