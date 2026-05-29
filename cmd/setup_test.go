package cmd

import (
	"io"
	"strings"
	"sync"
	"testing"
)

func resetScanner(input string) {
	stdinScannerOnce = sync.Once{}
	stdinScanner = nil
	stdinReader = func() io.Reader { return strings.NewReader(input) }
}

func TestPromptReturnsInput(t *testing.T) {
	resetScanner("myvalue\n")
	got := prompt("Label", "")
	if got != "myvalue" {
		t.Errorf("got %q, want %q", got, "myvalue")
	}
}

func TestPromptReturnsDefaultOnEmptyInput(t *testing.T) {
	resetScanner("\n")
	got := prompt("Label", "default")
	if got != "default" {
		t.Errorf("got %q, want default", got)
	}
}

func TestPromptHandlesCarriageReturn(t *testing.T) {
	resetScanner("myvalue\r")
	got := prompt("Label", "")
	if got != "myvalue" {
		t.Errorf("got %q, want %q (bare \\r not handled)", got, "myvalue")
	}
}

func TestPromptHandlesCRLF(t *testing.T) {
	resetScanner("myvalue\r\n")
	got := prompt("Label", "")
	if got != "myvalue" {
		t.Errorf("got %q, want %q (\\r\\n not handled)", got, "myvalue")
	}
}

func TestPromptMultipleCallsShareScanner(t *testing.T) {
	resetScanner("first\nsecond\nthird\n")
	a := prompt("A", "")
	b := prompt("B", "")
	c := prompt("C", "")
	if a != "first" || b != "second" || c != "third" {
		t.Errorf("got %q %q %q, want first second third", a, b, c)
	}
}

func TestScanAnyNewlineVariants(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []string
	}{
		{"unix LF", "a\nb\nc\n", []string{"a", "b", "c"}},
		{"windows CRLF", "a\r\nb\r\nc\r\n", []string{"a", "b", "c"}},
		{"bare CR", "a\rb\rc\r", []string{"a", "b", "c"}},
		{"mixed", "a\nb\r\nc\r", []string{"a", "b", "c"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetScanner(tc.input)
			for i, want := range tc.want {
				got := prompt("x", "")
				if got != want {
					t.Errorf("call %d: got %q, want %q", i+1, got, want)
				}
			}
		})
	}
}
