package auth

import (
	"testing"
	"yati/internal/config"
)

func TestGetToken_FromConfig(t *testing.T) {
	cfg := &config.Config{
		Token: "test-token",
	}

	token, err := GetToken(cfg)
	if err != nil {
		t.Fatalf("GetToken() failed: %v", err)
	}

	if token != "test-token" {
		t.Errorf("Expected token 'test-token', got '%s'", token)
	}
}

func TestGetToken_NoToken(t *testing.T) {
	cfg := &config.Config{}

	_, err := GetToken(cfg)
	if err == nil {
		t.Error("Expected error when no token and no pass path, got nil")
	}
}
