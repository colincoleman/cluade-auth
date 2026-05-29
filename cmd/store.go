package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ksgit/claude-auth/internal/config"
	"github.com/ksgit/claude-auth/internal/onepw"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var storeCmd = &cobra.Command{
	Use:   "store",
	Short: "Store long-term IAM credentials in 1Password",
	RunE:  runStore,
}

func runStore(_ *cobra.Command, _ []string) error {
	cfg, err := loadConfigOrSetup()
	if err != nil {
		return err
	}

	fmt.Printf("Storing credentials in 1Password vault %q, item %q\n", cfg.Vault, cfg.Item)
	fmt.Println()

	accessKeyID := prompt("AWS Access Key ID", "")
	if accessKeyID == "" {
		return fmt.Errorf("access key ID is required")
	}

	secretAccessKey, err := promptSecret("AWS Secret Access Key")
	if err != nil {
		return err
	}

	ctx := context.Background()
	client, err := onepw.New(ctx, cfg.OnePasswordAccount)
	if err != nil {
		return err
	}

	if err := client.StoreCredentials(ctx, cfg.Vault, cfg.Item, accessKeyID, secretAccessKey); err != nil {
		return err
	}

	if cfg.MFASerial != "" {
		fmt.Println()
		fmt.Printf("Reminder: the role %q requires MFA.\n", cfg.RoleARN)
		fmt.Printf("In the 1Password app, add a one-time-password (TOTP) field to the\n")
		fmt.Printf("%q item in vault %q using your MFA device's setup secret.\n", cfg.Item, cfg.Vault)
		fmt.Println("claude-auth reads the live code from it during refresh.")
	}
	return nil
}

func promptSecret(label string) (string, error) {
	fmt.Printf("%s: ", label)
	if term.IsTerminal(int(os.Stdin.Fd())) {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return "", fmt.Errorf("failed to read password: %w", err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	val := prompt(label, "")
	return val, nil
}

func loadConfigOrSetup() (*config.Config, error) {
	if !config.Exists() {
		fmt.Println("No config found. Running setup first...")
		fmt.Println()
		if err := runSetup(nil, nil); err != nil {
			return nil, err
		}
	}
	return config.Load()
}
