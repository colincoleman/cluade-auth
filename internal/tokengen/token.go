package tokengen

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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

// TokenInfo holds the metadata decoded from a token's embedded presigned URL.
type TokenInfo struct {
	Region string    // region the token is signed for
	Expiry time.Time // when the token stops being valid
}

// Decode parses a token (without making any network call) and extracts the
// region it is scoped to and its expiry, from the embedded SigV4 query params.
// This lets `claude-auth check` catch region mismatches and expiry locally.
func Decode(token string) (*TokenInfo, error) {
	if !strings.HasPrefix(token, tokenPrefix) {
		return nil, fmt.Errorf("not a valid Anthropic AWS API key (missing prefix)")
	}
	encoded := strings.TrimPrefix(token, tokenPrefix)
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("token payload is not valid base64: %w", err)
	}

	payload := strings.TrimSuffix(string(decoded), tokenVersion)
	// payload is "host/?query" — parse the query string
	q := payload
	if i := strings.Index(payload, "?"); i >= 0 {
		q = payload[i+1:]
	}
	values, err := url.ParseQuery(q)
	if err != nil {
		return nil, fmt.Errorf("could not parse token query: %w", err)
	}

	info := &TokenInfo{}

	// X-Amz-Credential = <access-key>/<date>/<region>/<service>/aws4_request
	cred := values.Get("X-Amz-Credential")
	if cred == "" {
		return nil, fmt.Errorf("region could not be extracted: X-Amz-Credential is missing")
	}
	parts := strings.Split(cred, "/")
	if len(parts) < 3 {
		return nil, fmt.Errorf("region could not be extracted: X-Amz-Credential has fewer than 3 segments")
	}
	info.Region = parts[2]

	// Expiry = X-Amz-Date + X-Amz-Expires
	if date := values.Get("X-Amz-Date"); date != "" {
		signed, err := time.Parse("20060102T150405Z", date)
		if err == nil {
			secs, _ := strconv.Atoi(values.Get("X-Amz-Expires"))
			info.Expiry = signed.Add(time.Duration(secs) * time.Second)
		}
	}

	return info, nil
}
