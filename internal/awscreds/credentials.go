package awscreds

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"gopkg.in/ini.v1"
)

func (s *SessionCredentials) ToAWSCredentials() (aws.Credentials, error) {
	return aws.Credentials{
		AccessKeyID:     s.AccessKeyID,
		SecretAccessKey: s.SecretAccessKey,
		SessionToken:    s.SessionToken,
		Expires:         s.Expiry,
		CanExpire:       true,
	}, nil
}

type SessionCredentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Expiry          time.Time
}

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

func WriteProfile(profile string, creds *SessionCredentials) error {
	path, err := credentialsFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	var f *ini.File
	if _, err := os.Stat(path); err == nil {
		f, err = ini.Load(path)
		if err != nil {
			return fmt.Errorf("failed to parse credentials file: %w", err)
		}
	} else {
		f = ini.Empty()
	}

	sec, err := f.NewSection(profile)
	if err != nil {
		sec = f.Section(profile)
	}
	sec.Key("aws_access_key_id").SetValue(creds.AccessKeyID)
	sec.Key("aws_secret_access_key").SetValue(creds.SecretAccessKey)
	sec.Key("aws_session_token").SetValue(creds.SessionToken)

	return f.SaveTo(path)
}

func LoadFromProfile(ctx context.Context, profile, region string) (aws.Config, error) {
	return config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(profile),
		config.WithRegion(region),
	)
}

func credentialsFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".aws", "credentials"), nil
}
