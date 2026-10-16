// Package gui provides Yati's optional Linux desktop interface.
package gui

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"yati/internal/config"
)

// Options controls the desktop app's launch behavior.
type Options struct {
	Background   bool
	Toggle       bool
	Quit         bool
	NoTray       bool
	startupReady func()
}

// Execute parses desktop options separately from the existing CLI's global flags.
func Execute(args []string, stdout, stderr io.Writer) int {
	startup, err := newStartupNotifier()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	defer startup.Close()
	flags := flag.NewFlagSet("yati gui", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var options Options
	var install, autostart, detach bool
	flags.BoolVar(&detach, "d", false, "Detach from the terminal")
	flags.BoolVar(&detach, "detach", false, "Detach from the terminal")
	flags.BoolVar(&options.Background, "background", false, "Start hidden in the tray")
	flags.BoolVar(&options.Toggle, "toggle", false, "Toggle the quick-entry window")
	flags.BoolVar(&options.Quit, "quit", false, "Quit the resident desktop app")
	flags.BoolVar(&options.NoTray, "no-tray", false, "Run without a tray icon")
	flags.BoolVar(&install, "install", false, "Install a per-user application launcher and icon")
	flags.BoolVar(&autostart, "autostart", false, "Install and enable startup at login")
	token := flags.String("token", "", "Toggl API token (or use your existing config/environment)")
	passPath := flags.String("pass-path", "", "Path to the token in pass")
	flags.Usage = func() {
		_, _ = fmt.Fprintln(flags.Output(), "Usage: yati gui [options]")
		_, _ = fmt.Fprintln(flags.Output(), "Open the existing quick-entry window or start a resident desktop app.")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}
	if flags.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, "Unexpected desktop arguments:", flags.Args())
		return 1
	}
	if options.Background && options.NoTray {
		_, _ = fmt.Fprintln(stderr, "--background requires a tray icon; omit --no-tray or open the window normally")
		return 1
	}
	if detach && (options.Quit || install || autostart) {
		_, _ = fmt.Fprintln(stderr, "--detach cannot be combined with --quit, --install, or --autostart")
		return 1
	}
	if install || autostart {
		if err := InstallDesktop(autostart); err != nil {
			_, _ = fmt.Fprintf(stderr, "Desktop installation failed: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintln(stdout, "Installed the Yati application launcher and icon.")
		if autostart {
			_, _ = fmt.Fprintln(stdout, "Yati will start in the tray at login.")
		}
		return 0
	}
	cfg, err := config.Load()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Error loading config: %v\n", err)
		return 1
	}
	if *token != "" {
		cfg.Token = *token
	}
	if *passPath != "" {
		cfg.PassPath = *passPath
	}
	if detach {
		logPath, err := detachDesktop(options, *token, *passPath)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "Yati desktop ready. Log: %s\n", logPath)
		return 0
	}
	options.startupReady = startup.Ready
	if err := runDesktop(cfg, options, stderr); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	startup.Ready() // A secondary invocation successfully activated the resident app.
	return 0
}
