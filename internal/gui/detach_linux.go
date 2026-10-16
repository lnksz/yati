//go:build linux && cgo && !nogui

package gui

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

func detachDesktop(options Options, token, passPath string) (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		state = filepath.Join(home, ".local", "state")
	}
	if !filepath.IsAbs(state) {
		return "", fmt.Errorf("XDG_STATE_HOME must be an absolute path")
	}
	logPath := filepath.Join(state, "yati", "gui.log")
	args := []string{"gui"}
	for _, flag := range []struct {
		enabled bool
		name    string
	}{{options.Background, "--background"}, {options.Toggle, "--toggle"}, {options.NoTray, "--no-tray"}} {
		if flag.enabled {
			args = append(args, flag.name)
		}
	}
	if token != "" {
		args = append(args, "--token", token)
	}
	if passPath != "" {
		args = append(args, "--pass-path", passPath)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err = startDetached(ctx, executable, args, os.Environ(), logPath)
	return logPath, err
}

func startDetached(ctx context.Context, executable string, args, environment []string, logPath string) (int, error) {
	if err := os.MkdirAll(filepath.Dir(logPath), 0700); err != nil {
		return 0, err
	}
	log, err := os.OpenFile(logPath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0600)
	if err != nil {
		return 0, err
	}
	defer func() { _ = log.Close() }() // The child retains its own duplicated descriptors.
	if err := log.Chmod(0600); err != nil {
		return 0, err
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		return 0, err
	}
	defer func() { _ = reader.Close(); _ = writer.Close() }() // Also covers failed exec.
	cmd := exec.Command(executable, args...)
	cmd.Env = append(environment, startupFDEnvironment+"=3")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout, cmd.Stderr = log, log
	cmd.ExtraFiles = []*os.File{writer}
	// Nil stdin becomes /dev/null; no terminal or output pipes survive the launcher.
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	if err := writer.Close(); err != nil {
		_ = cmd.Process.Kill() // Cleanup the child whose startup cannot be observed.
		_ = cmd.Wait()
		return 0, err
	}
	ready := make(chan bool, 1)
	go func() {
		line, err := bufio.NewReader(reader).ReadString('\n')
		ready <- err == nil && line == "ready\n"
	}()
	select {
	case ok := <-ready:
		if !ok {
			exited := make(chan error, 1)
			go func() { exited <- cmd.Wait() }()
			select {
			case err := <-exited:
				return 0, fmt.Errorf("desktop exited before becoming ready (%v); see %s", err, logPath)
			case <-ctx.Done():
				_ = cmd.Process.Kill() // Reap a child that closed startup IPC without becoming ready.
				<-exited
				return 0, fmt.Errorf("desktop startup: %w; see %s", ctx.Err(), logPath)
			}
		}
		pid := cmd.Process.Pid
		return pid, cmd.Process.Release()
	case <-ctx.Done():
		_ = cmd.Process.Kill() // Only the newly spawned child, never another desktop instance.
		_ = cmd.Wait()
		return 0, fmt.Errorf("desktop startup: %w; see %s", ctx.Err(), logPath)
	}
}
