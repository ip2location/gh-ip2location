package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Extension configuration
type Config struct {
	APIKey string `json:"api_key,omitempty"`
}

// Configuration path
func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "gh-ip2location", "config.json"), nil
}

// Load extension configuration
func loadConfig() Config {
	var cfg Config

	p, err := configPath()
	if err != nil {
		return cfg
	}

	b, err := os.ReadFile(p)
	if err != nil {
		return cfg
	}

	_ = json.Unmarshal(b, &cfg)
	return cfg
}

func saveConfig(cfg Config) error {
	p, err := configPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}

	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(p, append(b, '\n'), 0o600)
}
