package cli

import (
	"fmt"
	"os"
	"time"

	"yati/internal/toggl"
)

func runContinue(client *toggl.Client, args []string) {
	endDate := time.Now()
	startDate := endDate.Add(-7 * 24 * time.Hour)

	entries, err := client.GetTimeEntries(startDate, endDate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching recent time entries: %v\n", err)
		os.Exit(1)
	}

	if len(entries) == 0 {
		fmt.Println("No recent time entries found.")
		return
	}

	var lastEntry *toggl.TimeEntry
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if e.Duration > 0 { // duration > 0 means it's stopped
			if lastEntry == nil || e.Start.After(lastEntry.Start) {
				lastEntry = &e
			}
		}
	}

	if lastEntry == nil {
		fmt.Println("No recently stopped time entries found.")
		return
	}

	started, err := client.StartTimeEntry(lastEntry.WorkspaceID, lastEntry.ProjectID, lastEntry.Description)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error continuing time entry: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Continued: %s\n", started.Description)
}
