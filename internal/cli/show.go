package cli

import (
	"fmt"
	"os"
	"time"

	"yati/internal/toggl"
)

func runShow(client *toggl.Client, args []string) {
	entry, err := client.GetCurrentTimeEntry()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting current time entry: %v\n", err)
		os.Exit(1)
	}
	if entry == nil {
		fmt.Println("No running time entry.")
		return
	}

	name := entry.Description
	if name == "" {
		name = "(no description)"
	}

	since := entry.Start.Local()
	duration := time.Since(since)

	var project string
	if entry.ProjectID != nil {
		me, err := client.GetMe()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching user info: %v\n", err)
			os.Exit(1)
		}

		projects, err := client.GetProjects(me.DefaultWorkspace)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching projects: %v\n", err)
			os.Exit(1)
		}

		for _, p := range projects {
			if p.ID == *entry.ProjectID {
				project = p.Name
				break
			}
		}
	}

	if project == "" {
		project = "(no project)"
	}

	fmt.Printf("Name: %s\n", name)
	fmt.Printf("Since: %s\n", since.Format("2006-01-02 15:04:05"))
	fmt.Printf("Duration: %s\n", duration.Truncate(time.Second))
	fmt.Printf("Project: %s\n", project)
}
