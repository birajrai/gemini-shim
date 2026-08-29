package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Config represents application configuration parameters.
type Config struct {
	Port              int      `json:"port"`
	Host              string   `json:"host"`
	RetryAttempts     int      `json:"retry_attempts"`
	RetryDelaySec     int      `json:"retry_delay_sec"`
	RequestTimeoutSec int      `json:"request_timeout_sec"`
	GeminiBL          string   `json:"gemini_bl"`
	AuthUser          *string  `json:"auth_user"`
	XSRFToken         *string  `json:"xsrf_token"`
	DefaultModel      string   `json:"default_model"`
	LogRequests       bool     `json:"log_requests"`
	CookieFile        string   `json:"cookie_file"`
	Proxy             string   `json:"proxy"`
	APIKeys           []string `json:"api_keys"`
	TemporaryChats    bool     `json:"temporary_chats"`
}

var (
	globalConfig *Config
	configMutex  sync.RWMutex
)

// DefaultConfig returns a new Config with standard default values.
func DefaultConfig() *Config {
	return &Config{
		Port:              8081,
		Host:              "0.0.0.0",
		RetryAttempts:     3,
		RetryDelaySec:     2,
		RequestTimeoutSec: 180,
		GeminiBL:          "boq_assistant-bard-web-server_20260827.05_p0",
		AuthUser:          nil,
		XSRFToken:         nil,
		DefaultModel:      "gemini-3.6-flash",
		LogRequests:       true,
		CookieFile:        "",
		Proxy:             "",
		APIKeys:           []string{},
		TemporaryChats:    false,
	}
}

func init() {
	globalConfig = DefaultConfig()
}

// Get returns the global active configuration.
func Get() *Config {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return globalConfig
}

// Set sets the global active configuration.
func Set(cfg *Config) {
	configMutex.Lock()
	defer configMutex.Unlock()
	globalConfig = cfg
}

// LoadConfig loads configuration from a JSON file path and merges with defaults.
func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()
	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config JSON: %w", err)
	}

	Set(cfg)
	return cfg, nil
}

// FindConfig searches standard locations for a config file.
func FindConfig() string {
	if _, err := os.Stat("./config.json"); err == nil {
		return "./config.json"
	}

	home, err := os.UserHomeDir()
	if err == nil {
		userConfig := filepath.Join(home, ".config", "gemini-shim", "config.json")
		if _, err := os.Stat(userConfig); err == nil {
			return userConfig
		}
	}

	return ""
}

// Log prints formatted timestamped logs to stderr if LogRequests is enabled.
func Log(format string, v ...any) {
	cfg := Get()
	if cfg != nil && !cfg.LogRequests {
		return
	}
	ts := time.Now().Format("15:04:05")
	fmt.Fprintf(os.Stderr, "[%s] %s\n", ts, fmt.Sprintf(format, v...))
}
