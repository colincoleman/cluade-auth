package awscreds_test

import (
	"testing"
	"time"

	"github.com/ksgit/claude-auth/internal/awscreds"
)

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
	if creds.SessionToken != "ST" {
		t.Errorf("SessionToken: got %q, want ST", creds.SessionToken)
	}
	if !creds.CanExpire {
		t.Error("CanExpire should be true")
	}
}
