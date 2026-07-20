package auth

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"

	"yati/internal/config"
)

// GetToken resolves the Toggl API token in the following order:
// 1. Directly from Config (which already prioritized CLI flags > Env Vars > Config File)
// 2. By executing `pass <pass_path>` if a pass path is configured
func GetToken(cfg *config.Config) (string, error) {
	if cfg.Token != "" {
		return cfg.Token, nil
	}

	if cfg.PassPath != "" {
		token, err := runPass(cfg.PassPath)
		if err != nil {
			return "", err
		}
		if token != "" {
			return token, nil
		}
	}

	return "", errors.New("toggl API token not found (checked flags, env vars, config file, and pass)")
}

func runPass(path string) (string, error) {
	cmd := exec.Command("pass", "show", path)
	var out bytes.Buffer
	cmd.Stdout = &out
	
	if err := cmd.Run(); err != nil {
		return "", err
	}

	// pass often outputs a trailing newline
	return strings.TrimSpace(out.String()), nil
}
