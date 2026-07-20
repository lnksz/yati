package cli

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestFishCompletions_ContainsAllSubcommands(t *testing.T) {
	subcommands := []string{"start", "stop", "continue", "interactive", "list", "completion"}
	for _, subcmd := range subcommands {
		if !strings.Contains(fishCompletions, subcmd) {
			t.Errorf("Missing subcommand: %s", subcmd)
		}
	}
}

func TestFishCompletions_ContainsGlobalFlags(t *testing.T) {
	flags := []string{"-l token", "-l pass-path"}
	for _, flag := range flags {
		if !strings.Contains(fishCompletions, flag) {
			t.Errorf("Missing flag: %s", flag)
		}
	}
}

func TestFishCompletions_ContainsStartFlag(t *testing.T) {
	if !strings.Contains(fishCompletions, "-s p") {
		t.Error("Missing start -p flag")
	}
}

func TestFishCompletions_ContainsListPeriods(t *testing.T) {
	periods := []string{"d", "w", "m"}
	for _, p := range periods {
		if !strings.Contains(fishCompletions, p) {
			t.Errorf("Missing period: %s", p)
		}
	}
}

func TestFishCompletions_ValidSyntax(t *testing.T) {
	tmpFile := t.TempDir() + "/test_completion.fish"
	if err := os.WriteFile(tmpFile, []byte(fishCompletions), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	cmd := exec.Command("fish", "-n", tmpFile)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("Fish syntax check failed: %v\n%s", err, out)
	}
}

func TestFishCompletions_DisablesFileCompletions(t *testing.T) {
	if !strings.Contains(fishCompletions, "complete -c yati -f") {
		t.Error("Missing file completion disable")
	}
}
