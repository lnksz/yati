package auth

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"

	"yati/internal/config"
)

// GetToken resolves the Toggl API token in the following order:
// 1. Directly from Config (which already prioritized CLI flags > Env Vars > Config File)
// 2. By executing `pass <pass_path>` if a pass path is configured
func GetToken(cfg *config.Config) (string, error) {
	return GetTokenContext(context.Background(), cfg)
}

// GetTokenContext resolves credentials and cancels a waiting password provider
// when the desktop application exits.
func GetTokenContext(ctx context.Context, cfg *config.Config) (string, error) {
	if cfg.Token != "" {
		return cfg.Token, nil
	}

	if cfg.PassPath != "" {
		token, err := runPassContext(ctx, cfg.PassPath)
		if err != nil {
			return "", err
		}
		if token != "" {
			return token, nil
		}
	}

	return "", errors.New("toggl API token not found (checked flags, env vars, config file, and pass)")
}

func runPassContext(ctx context.Context, path string) (string, error) {
	cmd := exec.CommandContext(ctx, "pass", "show", path)
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return "", err
	}

	// pass often outputs a trailing newline
	return strings.TrimSpace(out.String()), nil
}
