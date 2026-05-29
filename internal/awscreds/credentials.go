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

// AssumeRole exchanges long-term IAM credentials for short-term credentials that
// hold the assumed role's permissions (e.g. aws-external-anthropic:CreateInference).
//
// It assumes the role directly from the long-term credentials in ONE step, passing
// the MFA SerialNumber + TokenCode. This "initial assumption from user credentials"
// is NOT role chaining, so it can last up to the role's MaxSessionDuration (12h).
// (The two-step GetSessionToken→AssumeRole pattern is role chaining and is hard-
// capped at 1h by AWS, so it is deliberately avoided here.)
//
// The returned credentials are used only to sign the presigned API-key token —
// never written to disk.
func AssumeRole(ctx context.Context, accessKeyID, secretAccessKey, roleARN, mfaSerial, tokenCode, region string, durationHours int) (*SessionCredentials, error) {
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, "")),
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
		// Don't auto-retry at a shorter duration: the MFA TokenCode is single-use,
		// so reusing it would fail. Give actionable guidance instead.
		if strings.Contains(err.Error(), "DurationSeconds") || strings.Contains(err.Error(), "MaxSessionDuration") {
			return nil, fmt.Errorf("the role's MaxSessionDuration is below the requested %dh — raise it (up to 12h):\n"+
				"  claude-auth aws-exec -- aws iam update-role --role-name <role> --max-session-duration %d\n"+
				"underlying error: %w", durationHours, durationHours*3600, err)
		}
		return nil, fmt.Errorf("STS AssumeRole failed: %w", err)
	}

	return &SessionCredentials{
		AccessKeyID:     aws.ToString(resp.Credentials.AccessKeyId),
		SecretAccessKey: aws.ToString(resp.Credentials.SecretAccessKey),
		SessionToken:    aws.ToString(resp.Credentials.SessionToken),
		Expiry:          aws.ToTime(resp.Credentials.Expiration),
	}, nil
}
