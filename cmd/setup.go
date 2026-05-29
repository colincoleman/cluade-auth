package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/ksgit/claude-auth/internal/config"
	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Interactive first-run configuration wizard",
	RunE:  runSetup,
}

// stdinScanner is a single shared scanner so that multiple prompt() calls
// don't each buffer separate chunks of stdin (which would swallow input).
var (
	stdinScanner     *bufio.Scanner
	stdinScannerOnce sync.Once
)

func getScanner() *bufio.Scanner {
	stdinScannerOnce.Do(func() {
		stdinScanner = bufio.NewScanner(stdinReader())
		stdinScanner.Split(scanAnyNewline)
	})
	return stdinScanner
}

// stdinReader is a variable so tests can swap os.Stdin for a pipe.
var stdinReader = func() io.Reader { return os.Stdin }

// scanAnyNewline is a bufio.SplitFunc that handles \n, \r\n, and bare \r.
// The default ScanLines only handles \n and \r\n, which breaks terminals
// (e.g. Claude Code's terminal) that send a bare \r on Enter.
func scanAnyNewline(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	for i, b := range data {
		switch b {
		case '\n':
			// Consume \r\n or bare \n
			line := data[:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			return i + 1, line, nil
		case '\r':
			if i+1 < len(data) {
				if data[i+1] == '\n' {
					return i + 2, data[:i], nil
				}
				return i + 1, data[:i], nil
			}
			// Need more data to distinguish \r from \r\n
			if atEOF {
				return i + 1, data[:i], nil
			}
			return 0, nil, nil
		}
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func runSetup(_ *cobra.Command, _ []string) error {
	fmt.Println("claude-auth setup")
	fmt.Println("─────────────────")
	fmt.Println("Press Enter to accept the default shown in [brackets].")
	fmt.Println()

	cfg := config.DefaultConfig()

	cfg.OnePasswordAccount = prompt("1Password account name (shown in app title bar)", "")
	if cfg.OnePasswordAccount == "" {
		return fmt.Errorf("1Password account name is required")
	}

	cfg.Vault = prompt("1Password vault", cfg.Vault)
	cfg.Item = prompt("1Password item name", cfg.Item)
	cfg.AWSProfile = prompt("AWS credentials profile", cfg.AWSProfile)
	cfg.AWSRegion = prompt("Preferred AWS region", cfg.AWSRegion)
	cfg.AWSRegionFallback = prompt("Fallback AWS region", cfg.AWSRegionFallback)
	cfg.WorkspaceID = prompt("Anthropic workspace ID (Claude Platform on AWS → Workspaces)", "")
	if cfg.WorkspaceID == "" {
		return fmt.Errorf("workspace ID is required")
	}

	if err := config.Save(&cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	path, _ := config.Path()
	fmt.Printf("\nConfig saved to %s\n", path)
	fmt.Println("\nNext steps:")
	fmt.Println("  1. In 1Password: Settings → Developer → enable \"Integrate with 1Password CLI\"")
	fmt.Println("  2. Run: claude-auth store")
	fmt.Println("  3. Run: claude-auth refresh")
	fmt.Printf("\nAdd to ~/.zshrc:\n")
	fmt.Println("  [ -f ~/.config/claude-auth/anthropic.env ] && source ~/.config/claude-auth/anthropic.env")
	fmt.Printf("  export ANTHROPIC_AWS_WORKSPACE_ID=%s\n", cfg.WorkspaceID)
	fmt.Println("  function claude() { AWS_PROFILE=" + cfg.AWSProfile + " command claude \"$@\"; }")

	return nil
}

// prompt prints a label and reads a line from stdin. If the user presses Enter
// with no input, defaultVal is returned (and shown in brackets in the prompt).
func prompt(label, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", label, defaultVal)
	} else {
		fmt.Printf("%s: ", label)
	}
	scanner := getScanner()
	if scanner.Scan() {
		val := strings.TrimSpace(scanner.Text())
		if val == "" {
			return defaultVal
		}
		return val
	}
	return defaultVal
}
