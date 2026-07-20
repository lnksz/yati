package main

import (
	"flag"
	"fmt"
	"os"

	"yati/internal/auth"
	"yati/internal/cli"
	"yati/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	cfg.ParseFlags()

	// Ensure we can get a token
	token, err := auth.GetToken(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting Toggl API token: %v\n", err)
		os.Exit(1)
	}

	// We use flag.Args() because flag.Parse() strips global flags
	cli.Execute(cfg, token, flag.Args())
}
