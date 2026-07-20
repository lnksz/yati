package cli

import (
	"fmt"
	"os"

	"yati/internal/config"
	"yati/internal/toggl"
)

func Execute(cfg *config.Config, token string, args []string) {
	if len(args) < 1 {
		printUsage()
		os.Exit(1)
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
		fmt.Printf("Unknown subcommand: %s\n", args[0])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: yati <subcommand> [flags]")
	fmt.Println("Subcommands:")
	fmt.Println("  start      Start a new time entry")
	fmt.Println("  stop       Stop the current time entry")
	fmt.Println("  continue   Continue the most recently stopped time entry")
	fmt.Println("  interactive Start a task interactively")
	fmt.Println("  list       List tasks for the day (d), work-week (w), or month (m)")
}
