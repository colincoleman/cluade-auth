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
// It follows aws-vault's two-step pattern: when MFA is required, first call
// GetSessionToken WITH the MFA code to mint an MFA-validated session, then call
// AssumeRole using that session (no token code on the assume). Some setups reject
// a one-step AssumeRole+TokenCode but accept this flow. The returned credentials
// are used only to sign the presigned API-key token — never written to disk.
func AssumeRole(ctx context.Context, accessKeyID, secretAccessKey, roleARN, mfaSerial, tokenCode, region string, durationHours int) (*SessionCredentials, error) {
	baseCfg, err := config.LoadDefaultConfig(ctx,
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, "")),
		config.WithRegion(region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build AWS config: %w", err)
	}
	base := sts.NewFromConfig(baseCfg)

	// Step 1: GetSessionToken (with MFA if required) → MFA-validated session.
	gstInput := &sts.GetSessionTokenInput{
		DurationSeconds: aws.Int32(int32(min(durationHours*3600, 129600))),
	}
	if mfaSerial != "" {
		gstInput.SerialNumber = aws.String(mfaSerial)
		gstInput.TokenCode = aws.String(tokenCode)
	}
	gst, err := base.GetSessionToken(ctx, gstInput)
	if err != nil {
		return nil, fmt.Errorf("STS GetSessionToken (MFA) failed: %w", err)
	}

	// Step 2: AssumeRole using the session creds — no token code needed; the
	// session already carries aws:MultiFactorAuthPresent=true.
	sessionCfg, err := config.LoadDefaultConfig(ctx,
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			aws.ToString(gst.Credentials.AccessKeyId),
			aws.ToString(gst.Credentials.SecretAccessKey),
			aws.ToString(gst.Credentials.SessionToken),
		)),
		config.WithRegion(region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build session AWS config: %w", err)
	}
	svc := sts.NewFromConfig(sessionCfg)

	input := &sts.AssumeRoleInput{
		RoleArn:         aws.String(roleARN),
		RoleSessionName: aws.String("claude-auth"),
		DurationSeconds: aws.Int32(int32(durationHours * 3600)),
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
