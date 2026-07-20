package main

import (
	"fmt"
	"os"

	"yati/internal/auth"
	"yati/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	cfg.ParseFlags()

	// Ensure we can get a token, but don't print it obviously
	_, err = auth.GetToken(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting Toggl API token: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("YATI initialized successfully. Implement subcommands next.")
}
