//go:build linux && cgo && !nogui

package gui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestDetachedHelper runs only in a subprocess. No GTK or Toggl requests occur.
func TestDetachedHelper(t *testing.T) {
	mode := os.Getenv("YATI_DETACH_TEST_MODE")
	if mode == "" {
		t.Skip("subprocess helper")
	}
	if mode == "fail" {
		_, _ = fmt.Fprintln(os.Stderr, "synthetic startup failure")
		os.Exit(23)
	}
	if mode == "ready" || mode == "closed" {
		startup, err := newStartupNotifier()
		if err != nil {
			t.Fatal(err)
		}
		if mode == "closed" {
			startup.Close()
			select {}
		}
		session, _, errno := syscall.Syscall(syscall.SYS_GETSID, 0, 0, 0)
		if errno != 0 {
			t.Fatal(errno)
		}
		info := map[string]int{"pid": os.Getpid(), "group": syscall.Getpgrp(), "session": int(session)}
		if err := json.NewEncoder(os.Stdout).Encode(info); err != nil {
			t.Fatal(err)
		}
		startup.Ready()
	}
	// Stay alive long enough to prove the launcher doesn't wait for desktop exit.
	select {}
}

func TestDetachedLaunch(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"ready", "fail", "timeout", "closed"} {
		t.Run(mode, func(t *testing.T) {
			logPath := filepath.Join(t.TempDir(), "gui.log")
			timeout := 5 * time.Second
			if mode == "timeout" || mode == "closed" {
				timeout = 250 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(t.Context(), timeout)
			defer cancel()
			pid, err := startDetached(ctx, executable, []string{"-test.run=^TestDetachedHelper$"}, append(os.Environ(), "YATI_DETACH_TEST_MODE="+mode), logPath)
			if mode == "fail" {
				if err == nil || !strings.Contains(err.Error(), "exit status 23") {
					t.Fatalf("startup failure not reported: %v", err)
				}
				data, readErr := os.ReadFile(logPath)
				if readErr != nil || !strings.Contains(string(data), "synthetic startup failure") {
					t.Fatalf("startup diagnostics were lost: %s (%v)", data, readErr)
				}
				return
			}
			if mode == "timeout" || mode == "closed" {
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("expected a bounded startup wait, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			child, err := os.FindProcess(pid)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				_ = child.Kill() // Only the synthetic helper belongs to this test.
				_, _ = child.Wait()
			}()
			cancel() // Ending the launch context must not kill a ready desktop.
			if err := child.Signal(syscall.Signal(0)); err != nil {
				t.Fatal("detached child did not survive the launcher:", err)
			}
			data, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatal(err)
			}
			var info map[string]int
			if err := json.Unmarshal(data, &info); err != nil {
				t.Fatal(err)
			}
			if info["pid"] != pid || info["group"] != pid || info["session"] != pid {
				t.Fatalf("child still belongs to the terminal's session: %v", info)
			}
			stat, err := os.Stat(logPath)
			if err != nil || stat.Mode().Perm() != 0600 {
				t.Fatalf("startup log isn't private: %v (%v)", stat, err)
			}
		})
	}
}
