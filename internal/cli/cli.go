package cli

import (
	"fmt"
	"io"

	"yati/internal/config"
	"yati/internal/toggl"
)

func Execute(cfg *config.Config, token string, args []string) {
	ExecuteTo(cfg, token, args, io.Discard, io.Discard)
}

func ExecuteTo(cfg *config.Config, token string, args []string, stdout, stderr io.Writer) {
	if len(args) < 1 {
		PrintUsage(stdout)
		return
	}

	client := toggl.NewClient(token)

	switch args[0] {
	case "start":
		runStart(client, args[1:])
	case "stop":
		runStop(client, args[1:])
	case "continue":
		runContinue(client, args[1:])
	case "interactive":
		runInteractiveStart(client, args[1:])
	case "list":
		runInteractiveList(client, args[1:])
	default:
		fmt.Fprintf(stderr, "Unknown subcommand: %s\n", args[0])
		PrintUsage(stderr)
	}
}

func PrintUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: yati <subcommand> [flags]")
	fmt.Fprintln(w, "Subcommands:")
	fmt.Fprintln(w, "  start       Start a new time entry")
	fmt.Fprintln(w, "  stop        Stop the current time entry")
	fmt.Fprintln(w, "  continue    Continue the most recently stopped time entry")
	fmt.Fprintln(w, "  interactive Start a task interactively")
	fmt.Fprintln(w, "  list        List tasks for the day (d), work-week (w), or month (m)")
	fmt.Fprintln(w, "  completion  Output shell completion script")
}
