package tokengen

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
)

const (
	serviceEndpoint = "https://aws-external-anthropic.amazonaws.com/"
	serviceHost     = "aws-external-anthropic.amazonaws.com"
	serviceName     = "aws-external-anthropic"
	tokenPrefix     = "aws-external-anthropic-api-key-"
	tokenVersion    = "&Version=1"
	emptyBodyHash   = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

// Generate creates a short-lived ANTHROPIC_AWS_API_KEY bearer token using the provided
// AWS credentials. The algorithm mirrors the official Python token generator:
// https://github.com/aws/token-generator-for-aws-external-anthropic-python
func Generate(ctx context.Context, creds aws.Credentials, region string, expiry time.Duration) (string, error) {
	req, err := http.NewRequest("POST", serviceEndpoint, nil)
	if err != nil {
		return "", fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("host", serviceHost)

	q := req.URL.Query()
	q.Set("Action", "CallWithBearerToken")
	q.Set("X-Amz-Expires", fmt.Sprintf("%d", int(expiry.Seconds())))
	req.URL.RawQuery = q.Encode()

	signer := v4.NewSigner()
	presignedURL, _, err := signer.PresignHTTP(
		ctx, creds, req, emptyBodyHash,
		serviceName, region,
		time.Now(),
	)
	if err != nil {
		return "", fmt.Errorf("failed to presign request: %w", err)
	}

	// Strip https:// prefix, append version, base64 encode
	stripped := presignedURL[len("https://"):]
	payload := stripped + tokenVersion
	encoded := base64.StdEncoding.EncodeToString([]byte(payload))

	return tokenPrefix + encoded, nil
}
