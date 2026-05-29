package awscreds

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

type SessionCredentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Expiry          time.Time
}

func (s *SessionCredentials) ToAWSCredentials() (aws.Credentials, error) {
	return aws.Credentials{
		AccessKeyID:     s.AccessKeyID,
		SecretAccessKey: s.SecretAccessKey,
		SessionToken:    s.SessionToken,
		Expires:         s.Expiry,
		CanExpire:       true,
	}, nil
}

// AssumeRole exchanges long-term IAM credentials for short-term credentials by
// assuming roleARN. The role must hold aws-external-anthropic:CreateInference.
// If mfaSerial is set, tokenCode (a TOTP) is supplied for the MFA challenge.
// The returned credentials are used only to sign the presigned API-key token —
// they are never written to disk.
func AssumeRole(ctx context.Context, accessKeyID, secretAccessKey, roleARN, mfaSerial, tokenCode, region string, durationHours int) (*SessionCredentials, error) {
	staticCreds := credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, "")
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithCredentialsProvider(staticCreds),
		config.WithRegion(region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build AWS config: %w", err)
	}

	svc := sts.NewFromConfig(cfg)

	input := &sts.AssumeRoleInput{
		RoleArn:         aws.String(roleARN),
		RoleSessionName: aws.String("claude-auth"),
		DurationSeconds: aws.Int32(int32(durationHours * 3600)),
	}
	if mfaSerial != "" {
		input.SerialNumber = aws.String(mfaSerial)
		input.TokenCode = aws.String(tokenCode)
	}

	resp, err := svc.AssumeRole(ctx, input)
	if err != nil {
		// The role's MaxSessionDuration may be shorter than requested; retry at 1h.
		if strings.Contains(err.Error(), "DurationSeconds") || strings.Contains(err.Error(), "MaxSessionDuration") {
			input.DurationSeconds = aws.Int32(3600)
			resp, err = svc.AssumeRole(ctx, input)
		}
		if err != nil {
			return nil, fmt.Errorf("STS AssumeRole failed: %w", err)
		}
	}

	return &SessionCredentials{
		AccessKeyID:     aws.ToString(resp.Credentials.AccessKeyId),
		SecretAccessKey: aws.ToString(resp.Credentials.SecretAccessKey),
		SessionToken:    aws.ToString(resp.Credentials.SessionToken),
		Expiry:          aws.ToTime(resp.Credentials.Expiration),
	}, nil
}
