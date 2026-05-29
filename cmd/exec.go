package cmd

import (
	"bufio"
	"fmt"
	"os"
	goexec "os/exec"
	"strings"
	"syscall"

	"github.com/ksgit/claude-auth/internal/config"
	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:   "exec -- <command> [args...]",
	Short: "Run a command in Claude Platform on AWS mode",
	Long: `Run a command with AWS Platform environment variables injected.
The current process is replaced (exec), so signals work correctly and
interactive tools like 'claude' behave as normal.

Examples:
  claude-auth exec -- claude           # run Claude Code in AWS Platform mode
  claude-auth exec -- $SHELL           # open an AWS Platform shell session
  claude-auth exec -- claude "prompt"  # pass arguments through`,
	Args: cobra.ArbitraryArgs,
	RunE: runExec,
}

func runExec(_ *cobra.Command, args []string) error {
	// Strip a leading "--" separator if present
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		return fmt.Errorf("no command specified — usage: claude-auth exec -- <command> [args...]")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Build the injected env vars (appended last so they override anything already set)
	inject := []string{
		"CLAUDE_CODE_USE_ANTHROPIC_AWS=1",
		"AWS_PROFILE=" + cfg.AWSProfile,
		"AWS_REGION=" + cfg.AWSRegion,
		"ANTHROPIC_AWS_WORKSPACE_ID=" + cfg.WorkspaceID,
	}

	if apiKey := readAnthropicAPIKey(); apiKey != "" {
		inject = append(inject, "ANTHROPIC_AWS_API_KEY="+apiKey)
	}

	env := append(os.Environ(), inject...)

	// Resolve the command binary
	command := args[0]
	path, err := goexec.LookPath(command)
	if err != nil {
		return fmt.Errorf("command %q not found: %w", command, err)
	}

	// syscall.Exec replaces the current process — no wrapper overhead,
	// correct signal handling, and interactive TUIs work properly.
	return syscall.Exec(path, args, env)
}

// readAnthropicAPIKey reads ANTHROPIC_AWS_API_KEY from anthropic.env if it exists.
func readAnthropicAPIKey() string {
	envPath, err := config.EnvPath()
	if err != nil {
		return ""
	}
	f, err := os.Open(envPath)
	if err != nil {
		return "" // file doesn't exist yet — fine, SigV4 will be used
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "ANTHROPIC_AWS_API_KEY=") {
			return strings.TrimPrefix(line, "ANTHROPIC_AWS_API_KEY=")
		}
	}
	return ""
}
