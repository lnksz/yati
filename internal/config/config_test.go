package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_EnvVars(t *testing.T) {
	// Setup env vars
	os.Setenv("TOGGL_API_TOKEN", "env-token")
	os.Setenv("YATI_PASS_PATH", "env-pass-path")
	defer os.Unsetenv("TOGGL_API_TOKEN")
	defer os.Unsetenv("YATI_PASS_PATH")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.Token != "env-token" {
		t.Errorf("Expected token to be 'env-token', got '%s'", cfg.Token)
	}
	if cfg.PassPath != "env-pass-path" {
		t.Errorf("Expected pass_path to be 'env-pass-path', got '%s'", cfg.PassPath)
	}
}

func TestLoadConfig_File(t *testing.T) {
	// Create a temp file
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "config.json")
	err := os.WriteFile(cfgFile, []byte(`{"token": "file-token", "pass_path": "file-pass-path"}`), 0644)
	if err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	os.Setenv("YATI_CONFIG", cfgFile)
	defer os.Unsetenv("YATI_CONFIG")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.Token != "file-token" {
		t.Errorf("Expected token to be 'file-token', got '%s'", cfg.Token)
	}
	if cfg.PassPath != "file-pass-path" {
		t.Errorf("Expected pass_path to be 'file-pass-path', got '%s'", cfg.PassPath)
	}
}

func TestLoadConfig_MergeFileAndEnv(t *testing.T) {
	// Env should override file for token, but if env is empty for pass_path, file should be used
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "config.json")
	err := os.WriteFile(cfgFile, []byte(`{"token": "file-token", "pass_path": "file-pass-path"}`), 0644)
	if err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	os.Setenv("YATI_CONFIG", cfgFile)
	defer os.Unsetenv("YATI_CONFIG")

	os.Setenv("TOGGL_API_TOKEN", "env-token")
	defer os.Unsetenv("TOGGL_API_TOKEN")
	// Note: YATI_PASS_PATH is not set in env

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.Token != "env-token" {
		t.Errorf("Expected token to be 'env-token', got '%s'", cfg.Token)
	}
	if cfg.PassPath != "file-pass-path" {
		t.Errorf("Expected pass_path to be 'file-pass-path', got '%s'", cfg.PassPath)
	}
}
