package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"yati/internal/toggl"
)

func runStart(client *toggl.Client, args []string) {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	projectIDFlag := fs.Int64("p", 0, "Project ID")

	fs.Parse(args)

	description := strings.Join(fs.Args(), " ")
	if description == "" {
		fmt.Println("Description is required.")
		os.Exit(1)
	}

	me, err := client.GetMe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching user info: %v\n", err)
		os.Exit(1)
	}

	workspaceID := me.DefaultWorkspace

	var pID *int64
	if *projectIDFlag != 0 {
		pid := *projectIDFlag
		pID = &pid
	}

	entry, err := client.StartTimeEntry(workspaceID, pID, description)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting time entry: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Started: %s\n", entry.Description)
}
