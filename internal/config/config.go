package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Cookie    string `json:"cookie"`
	StatsigID string `json:"statsig_id"`
	Model     string `json:"model"`
}

func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "grok-tui.json"
	}
	return filepath.Join(home, ".config", "grok-tui", "config.json")
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config file not found at %s: %w\n\nRun 'grok-tui init' to create a config file", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid config JSON: %w", err)
	}

	if cfg.Cookie == "" {
		return nil, fmt.Errorf("cookie is required in config file")
	}

	if cfg.Model == "" {
		cfg.Model = "grok-3"
	}

	return &cfg, nil
}

func Save(path string, cfg *Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

func CreateDefault(path string) error {
	cfg := &Config{
		Cookie:    "",
		StatsigID: "",
		Model:     "grok-3",
	}
	return Save(path, cfg)
}
