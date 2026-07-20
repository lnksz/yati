package config

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
)

type Config struct {
	Token    string `json:"token"`
	PassPath string `json:"pass_path"`
}

func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "yati", "config.json")
}

func Load() (*Config, error) {
	cfg := &Config{}

	// 1. Read from config file
	cfgPath := DefaultConfigPath()
	if envCfgPath := os.Getenv("YATI_CONFIG"); envCfgPath != "" {
		cfgPath = envCfgPath
	}

	if cfgPath != "" {
		data, err := os.ReadFile(cfgPath)
		if err == nil {
			_ = json.Unmarshal(data, cfg)
		} else if !os.IsNotExist(err) {
			// If error is something other than NotExist, we might want to return it,
			// but for simplicity we can just log/ignore or return. Let's return.
			return nil, err
		}
	}

	// 2. Read from Environment Variables
	if token := os.Getenv("TOGGL_API_TOKEN"); token != "" {
		cfg.Token = token
	}
	if passPath := os.Getenv("YATI_PASS_PATH"); passPath != "" {
		cfg.PassPath = passPath
	}

	return cfg, nil
}

// ParseFlags is separated to allow main to define the flags and pass them down
func (c *Config) ParseFlags() {
	tokenFlag := flag.String("token", "", "Toggl API token")
	passPathFlag := flag.String("pass-path", "", "Path to the Toggl API token in pass (password-store)")

	flag.Parse()

	// 3. Command line flags override everything
	if *tokenFlag != "" {
		c.Token = *tokenFlag
	}
	if *passPathFlag != "" {
		c.PassPath = *passPathFlag
	}
}
