package awscreds

import (
	"context"
	"fmt"
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

// GetSessionToken exchanges long-term IAM credentials for short-term STS
// session credentials. These are used only to sign the presigned API-key
// token — they are never written to disk.
func GetSessionToken(ctx context.Context, accessKeyID, secretAccessKey, region string, durationHours int) (*SessionCredentials, error) {
	staticCreds := credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, "")
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithCredentialsProvider(staticCreds),
		config.WithRegion(region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build AWS config: %w", err)
	}

	svc := sts.NewFromConfig(cfg)
	duration := int32(durationHours * 3600)
	resp, err := svc.GetSessionToken(ctx, &sts.GetSessionTokenInput{
		DurationSeconds: aws.Int32(duration),
	})
	if err != nil {
		return nil, fmt.Errorf("STS GetSessionToken failed: %w", err)
	}

	return &SessionCredentials{
		AccessKeyID:     aws.ToString(resp.Credentials.AccessKeyId),
		SecretAccessKey: aws.ToString(resp.Credentials.SecretAccessKey),
		SessionToken:    aws.ToString(resp.Credentials.SessionToken),
		Expiry:          aws.ToTime(resp.Credentials.Expiration),
	}, nil
}
