package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"yati/internal/auth"
	"yati/internal/cli"
	"yati/internal/config"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		cli.PrintUsage(stdout)
		return 0
	}

	if args[0] == "-h" || args[0] == "--help" {
		cli.PrintUsage(stdout)
		return 0
	}

	if args[0] == "completion" {
		return runCompletion(args[1:], stdout, stderr)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(stderr, "Error loading config: %v\n", err)
		return 1
	}

	cfg.ParseFlags()

	token, err := auth.GetToken(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "Error getting Toggl API token: %v\n", err)
		return 1
	}

	cli.ExecuteTo(cfg, token, flag.Args(), stdout, stderr)
	return 0
}

func runCompletion(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(stderr, "Usage: yati completion <shell>")
		fmt.Fprintln(stderr, "Supported shells: fish")
		return 1
	}

	switch args[0] {
	case "fish":
		cli.PrintFishCompletions(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "Unsupported shell: %s\n", args[0])
		fmt.Fprintln(stderr, "Supported shells: fish")
		return 1
	}
}
