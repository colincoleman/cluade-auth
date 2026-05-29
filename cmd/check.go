package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/ksgit/claude-auth/internal/awscreds"
	"github.com/ksgit/claude-auth/internal/config"
	"github.com/spf13/cobra"
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Verify the claude-auth setup is working correctly",
	RunE:  runCheck,
}

func runCheck(_ *cobra.Command, _ []string) error {
	fmt.Println("claude-auth check")
	fmt.Println("─────────────────")

	// 1. Config
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("✗ Config        %v\n", err)
		return err
	}
	path, _ := config.Path()
	fmt.Printf("✓ Config        %s\n", path)
	fmt.Printf("  Profile:      [%s]  Workspace region: %s\n", cfg.AWSProfile, cfg.EffectiveWorkspaceRegion())

	// 2. AWS identity via STS GetCallerIdentity
	ctx := context.Background()
	awsCfg, err := awscreds.LoadFromProfile(ctx, cfg.AWSProfile, cfg.AWSRegion)
	if err != nil {
		fmt.Printf("✗ AWS identity  could not load profile [%s]: %v\n", cfg.AWSProfile, err)
		fmt.Println("  → Run 'claude-auth refresh' to obtain credentials")
	} else {
		svc := sts.NewFromConfig(awsCfg)
		identity, err := svc.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
		if err != nil {
			fmt.Printf("✗ AWS identity  STS call failed: %v\n", err)
			fmt.Println("  → Credentials may be expired — run 'claude-auth refresh'")
		} else {
			fmt.Printf("✓ AWS identity  %s\n", deref(identity.Arn))
			fmt.Println("  (If you see a 403 from 'claude-auth exec', attach the")
			fmt.Println("   AnthropicInferenceAccess managed policy to this principal in AWS IAM)")
		}
	}

	// 3. Token expiry
	state, _ := config.LoadState()
	if state == nil || state.AWSExpiry == "" {
		fmt.Println("✗ Token expiry  not set — run 'claude-auth refresh'")
	} else {
		t, err := time.Parse(time.RFC3339, state.AWSExpiry)
		if err != nil {
			fmt.Println("✗ Token expiry  unreadable")
		} else {
			remaining := time.Until(t)
			if remaining <= 0 {
				fmt.Println("✗ Token expiry  EXPIRED — run 'claude-auth refresh'")
			} else if remaining < 30*time.Minute {
				h := int(remaining.Minutes())
				fmt.Printf("⚠ Token expiry  %dm remaining — consider refreshing soon\n", h)
			} else {
				h := int(remaining.Hours())
				m := int(remaining.Minutes()) % 60
				fmt.Printf("✓ Token expiry  %dh %dm remaining\n", h, m)
			}
		}
	}

	// 4. Anthropic key file
	envPath, _ := config.EnvPath()
	info, err := os.Stat(envPath)
	if err != nil || info.Size() == 0 {
		fmt.Println("✗ Anthropic key ~/.config/claude-auth/anthropic.env not found")
		fmt.Println("  → Run 'claude-auth refresh'")
	} else {
		fmt.Printf("✓ Anthropic key %s\n", envPath)
	}

	return nil
}

func deref(s *string) string {
	if s == nil {
		return "(unknown)"
	}
	return *s
}
