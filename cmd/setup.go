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
	"golang.org/x/term"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Interactive first-run configuration wizard",
	RunE:  runSetup,
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
	fmt.Println("\nAdd to ~/.zshrc:")
	fmt.Println("  [ -f ~/.config/claude-auth/anthropic.env ] && source ~/.config/claude-auth/anthropic.env")
	fmt.Printf("  export ANTHROPIC_AWS_WORKSPACE_ID=%s\n", cfg.WorkspaceID)
	fmt.Println("  function claude() { AWS_PROFILE=" + cfg.AWSProfile + " command claude \"$@\"; }")

	return nil
}

// prompt prints a label and reads a line from stdin.
// On an interactive terminal it uses raw mode so that it handles both \r and \n
// as Enter — fixing terminals (e.g. Claude Code) that send \r instead of \n.
// On a non-terminal (pipe, test) it falls back to a buffered scanner.
func prompt(label, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", label, defaultVal)
	} else {
		fmt.Printf("%s: ", label)
	}

	var val string
	if term.IsTerminal(int(os.Stdin.Fd())) {
		val = readLineRaw()
	} else {
		val = readLineScanner()
	}

	val = strings.TrimSpace(val)
	if val == "" {
		return defaultVal
	}
	return val
}

// readLineRaw reads one line from stdin in raw mode so that we receive \r
// directly (instead of waiting for the TTY line discipline to see \n).
// Handles backspace/DEL and Ctrl+C. Echoes printable ASCII characters.
func readLineRaw() string {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		// Can't get raw mode — fall back to buffered scanner.
		return readLineScanner()
	}
	defer term.Restore(fd, oldState)

	var buf []byte
	b := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(b)
		if n == 0 || err != nil {
			break
		}
		switch b[0] {
		case '\r', '\n':
			// In raw mode \n isn't translated to CRLF automatically.
			os.Stdout.WriteString("\r\n")
			return string(buf)
		case 127, '\b': // DEL or backspace
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
				os.Stdout.WriteString("\b \b")
			}
		case 3: // Ctrl+C
			os.Stdout.WriteString("^C\r\n")
			term.Restore(fd, oldState)
			os.Exit(1)
		default:
			if b[0] >= 32 && b[0] < 127 { // printable ASCII
				buf = append(buf, b[0])
				os.Stdout.Write(b[:1])
			}
		}
	}
	return string(buf)
}

// ── non-terminal (pipe / test) path ─────────────────────────────────────────

// stdinScanner is shared so that multiple prompt() calls don't each buffer
// separate chunks of stdin (which would swallow subsequent input lines).
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

func readLineScanner() string {
	scanner := getScanner()
	if scanner.Scan() {
		return scanner.Text()
	}
	return ""
}

// scanAnyNewline handles \n, \r\n, and bare \r for piped/test input.
func scanAnyNewline(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	for i, b := range data {
		switch b {
		case '\n':
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
