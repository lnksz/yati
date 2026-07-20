package cli

import (
	"fmt"
	"os"

	"yati/internal/toggl"
)

func runStop(client *toggl.Client, args []string) {
	entry, err := client.GetCurrentTimeEntry()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting current time entry: %v\n", err)
		os.Exit(1)
	}
	if entry == nil {
		fmt.Println("No running time entry to stop.")
		return
	}

	stopped, err := client.StopTimeEntry(entry.WorkspaceID, entry.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping time entry: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Stopped: %s (Duration: %ds)\n", stopped.Description, stopped.Duration)
}
