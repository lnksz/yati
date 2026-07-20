package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintUsage_ShowsAliases(t *testing.T) {
	var buf bytes.Buffer
	PrintUsage(&buf)
	out := buf.String()

	tests := []struct {
		name    string
		alias   string
		pattern string
	}{
		{"start alias", "s", "(s)"},
		{"stop alias", "S", "(S)"},
		{"continue alias", "c", "(c)"},
		{"show alias", "w", "(w)"},
		{"list alias", "ls", "(ls)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(out, tt.pattern) {
				t.Errorf("usage missing alias %s: got %s", tt.pattern, out)
			}
		})
	}
}

func TestExecuteTo_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	ExecuteTo(nil, "", []string{"nope"}, &stdout, &stderr)
	if !strings.Contains(stderr.String(), "Unknown subcommand: nope") {
		t.Errorf("expected unknown subcommand error, got stderr: %s", stderr.String())
	}
}

func TestExecuteTo_ShowsUsageOnNoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	ExecuteTo(nil, "", []string{}, &stdout, &stderr)
	if !strings.Contains(stdout.String(), "Usage: yati <subcommand>") {
		t.Errorf("expected usage output, got stdout: %s", stdout.String())
	}
}
