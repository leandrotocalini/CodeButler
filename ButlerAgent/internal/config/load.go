package config

import (
	"encoding/json"
	"fmt"
	"os"
)

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func validate(cfg *Config) error {
	if cfg.WhatsApp.SessionPath == "" {
		return fmt.Errorf("whatsapp.sessionPath is required")
	}

	if cfg.Sources.RootPath == "" {
		return fmt.Errorf("sources.rootPath is required")
	}

	return nil
}
