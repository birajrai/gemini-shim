package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/birajrai/gemini-shim/internal/config"
)

func TestDefaultConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	if cfg.Port != 8081 {
		t.Errorf("expected port 8081, got %d", cfg.Port)
	}
	if cfg.DefaultModel != "gemini-3.6-flash" {
		t.Errorf("expected default model gemini-3.6-flash, got %s", cfg.DefaultModel)
	}
	if cfg.TemporaryChats {
		t.Errorf("expected temporary_chats default to false")
	}
}

func TestLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	confPath := filepath.Join(tmpDir, "config.json")
	content := `{"port": 9090, "default_model": "gemini-3.7-flash", "temporary_chats": true, "api_keys": ["secret-123"]}`
	if err := os.WriteFile(confPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write tmp config: %v", err)
	}

	cfg, err := config.LoadConfig(confPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Port)
	}
	if cfg.DefaultModel != "gemini-3.7-flash" {
		t.Errorf("expected model gemini-3.7-flash, got %s", cfg.DefaultModel)
	}
	if !cfg.TemporaryChats {
		t.Errorf("expected temporary_chats to be true")
	}
	if len(cfg.APIKeys) != 1 || cfg.APIKeys[0] != "secret-123" {
		t.Errorf("unexpected api keys: %v", cfg.APIKeys)
	}
}
