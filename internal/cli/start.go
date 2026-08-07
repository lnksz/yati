package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"yati/internal/toggl"
)

func runStart(client *toggl.Client, args []string) {
	projectArg, picker, args := parseStartArgs(args)

	description := strings.Join(args, " ")
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
	var projName string

	if projectArg != "" {
		pid, name, err := resolveProject(client, workspaceID, projectArg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching projects: %v\n", err)
			os.Exit(1)
		}
		if pid != nil {
			pID = pid
			projName = name
		} else {
			fmt.Fprintf(os.Stderr, "Warning: project %q not found, starting without a project.\n", projectArg)
		}
	} else if picker {
		pid, name, cancelled, err := pickProject(client, workspaceID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error picking project: %v\n", err)
			os.Exit(1)
		}
		if cancelled {
			fmt.Println("No project selected, aborting.")
			return
		}
		pID = pid
		projName = name
	}

	entry, err := client.StartTimeEntry(workspaceID, pID, description)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting time entry: %v\n", err)
		os.Exit(1)
	}

	if projName != "" {
		fmt.Printf("Started: %s (project: %s)\n", entry.Description, projName)
	} else {
		fmt.Printf("Started: %s\n", entry.Description)
	}
}

// parseStartArgs splits the project flag (-p/--p) out of args. The flag takes
// an optional value: -p name uses name as the project, while a bare -p asks
// for an interactive picker. Everything else is returned as positional args.
func parseStartArgs(args []string) (project string, picker bool, rest []string) {
	rest = make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-p" || arg == "--p":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				project = args[i+1]
				i++
			} else {
				picker = true
			}
		case strings.HasPrefix(arg, "-p=") || strings.HasPrefix(arg, "--p="):
			project = strings.TrimPrefix(strings.TrimPrefix(arg, "--p="), "-p=")
		default:
			rest = append(rest, arg)
		}
	}
	return project, picker, rest
}

// resolveProject turns a -p argument into a project ID and name, or (nil, "")
// if no matching project exists. numeric arguments match by project ID so the
// old Project ID behaviour keeps working; everything else matches by name.
func resolveProject(client *toggl.Client, workspaceID int64, arg string) (*int64, string, error) {
	projects, err := client.GetProjects(workspaceID)
	if err != nil {
		return nil, "", err
	}
	pid, name := findProject(projects, arg)
	return pid, name, nil
}

func findProject(projects []toggl.Project, arg string) (*int64, string) {
	if id, err := strconv.ParseInt(arg, 10, 64); err == nil {
		for _, p := range projects {
			if p.ID == id {
				return &id, p.Name
			}
		}
		return nil, ""
	}

	for _, p := range projects {
		if p.Name == arg {
			pid := p.ID
			return &pid, p.Name
		}
	}

	for _, p := range projects {
		if strings.EqualFold(p.Name, arg) {
			pid := p.ID
			return &pid, p.Name
		}
	}
	return nil, ""
}
