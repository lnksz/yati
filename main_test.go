package main

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func runWithEnv(args []string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	rc := run(args, &stdout, &stderr)
	return rc, stdout.String(), stderr.String()
}

func TestRun_NoArgs_ShowsUsage(t *testing.T) {
	rc, out, _ := runWithEnv(nil)
	if rc != 0 {
		t.Fatalf("expected exit 0, got %d", rc)
	}
	if !strings.Contains(out, "Usage: yati <subcommand>") {
		t.Errorf("expected usage output, got: %s", out)
	}
}

func TestRun_Help_ShowsUsage(t *testing.T) {
	for _, flag := range []string{"-h", "--help"} {
		rc, out, _ := runWithEnv([]string{flag})
		if rc != 0 {
			t.Fatalf("%s: expected exit 0, got %d", flag, rc)
		}
		if !strings.Contains(out, "Usage: yati <subcommand>") {
			t.Errorf("%s: expected usage output, got: %s", flag, out)
		}
	}
}

func TestRun_CompletionFish_NoTokenNeeded(t *testing.T) {
	rc, out, err := runWithEnv([]string{"completion", "fish"})
	if rc != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", rc, err)
	}
	if !strings.Contains(out, "complete -c yati") {
		t.Error("expected fish completion output")
	}
}

func TestRun_CompletionFish_SyntaxValid(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping fish syntax check")
	}
	rc, out, _ := runWithEnv([]string{"completion", "fish"})
	if rc != 0 {
		t.Fatalf("unexpected non-zero exit: %d", rc)
	}
	tmpFile := t.TempDir() + "/test_completion.fish"
	if err := os.WriteFile(tmpFile, []byte(out), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	cmd := exec.Command("fish", "-n", tmpFile)
	if combinedOut, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("fish syntax check failed: %v\n%s", err, combinedOut)
	}
}

func TestRun_CompletionNoShell_ShowsUsageError(t *testing.T) {
	rc, out, err := runWithEnv([]string{"completion"})
	if rc != 1 {
		t.Fatalf("expected exit 1, got %d", rc)
	}
	if !strings.Contains(err, "Usage: yati completion <shell>") {
		t.Errorf("expected usage error, got stderr: %s", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("expected no stdout, got: %s", out)
	}
}

func TestRun_CompletionUnsupportedShell(t *testing.T) {
	rc, _, err := runWithEnv([]string{"completion", "bash"})
	if rc != 1 {
		t.Fatalf("expected exit 1, got %d", rc)
	}
	if !strings.Contains(err, "Unsupported shell: bash") {
		t.Errorf("expected unsupported shell error, got stderr: %s", err)
	}
}
