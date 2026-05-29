package cmd

import (
	"context"
	"fmt"
	"os"
	goexec "os/exec"
	"syscall"

	"github.com/ksgit/claude-auth/internal/config"
	"github.com/spf13/cobra"
)

var awsExecCmd = &cobra.Command{
	Use:   "aws-exec -- <command> [args...]",
	Short: "Run a command with short-term AWS credentials from the assumed role",
	Long: `Assume the configured role (1Password long-term creds + MFA) and run a
command with the resulting short-term AWS credentials exported as
AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY / AWS_SESSION_TOKEN.

Nothing is persisted to ~/.aws/credentials. Useful for one-off admin tasks
without keeping long-term keys on disk.

Examples:
  claude-auth aws-exec -- aws sts get-caller-identity
  claude-auth aws-exec -- aws iam create-role --role-name claude-platform ...`,
	Args: cobra.ArbitraryArgs,
	RunE: runAWSExec,
}

func runAWSExec(_ *cobra.Command, args []string) error {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		return fmt.Errorf("no command specified — usage: claude-auth aws-exec -- <command> [args...]")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx := context.Background()
	creds, err := assumeConfiguredRole(ctx, cfg)
	if err != nil {
		return err
	}

	inject := []string{
		"AWS_ACCESS_KEY_ID=" + creds.AccessKeyID,
		"AWS_SECRET_ACCESS_KEY=" + creds.SecretAccessKey,
		"AWS_SESSION_TOKEN=" + creds.SessionToken,
		"AWS_REGION=" + cfg.WorkspaceRegion,
	}
	env := append(os.Environ(), inject...)

	path, err := goexec.LookPath(args[0])
	if err != nil {
		return fmt.Errorf("command %q not found: %w", args[0], err)
	}
	return syscall.Exec(path, args, env)
}
