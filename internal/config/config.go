// Package config manages the persistent hermes-go configuration stored at
// ~/.hermes-go/config.json. It is read by the LLM client as a fallback when
// environment variables are absent, and written by the --configure wizard.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config is the persisted configuration for hermes-go.
// Fields map directly to the environment variables they shadow:
//
//	BaseURL → OPENAI_BASE_URL
//	APIKey  → OPENAI_API_KEY
//	Model   → OPENAI_MODEL
type Config struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
}

// DefaultPath returns the canonical config file path: ~/.hermes-go/config.json.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("config: cannot locate home directory: %w", err)
	}
	return filepath.Join(home, ".hermes-go", "config.json"), nil
}

// Load reads the config file at path and returns its contents.
// If the file does not exist, Load returns an empty Config and no error —
// a missing file is treated as "unconfigured", not as a failure.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("config: read %s: %w", path, err)
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("config: parse %s: %w", path, err)
	}
	return c, nil
}

// Save writes c to path as indented JSON, creating parent directories as
// needed. The file is written with mode 0600 (owner-read/write only) because
// it contains the API key.
func Save(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("config: mkdir %s: %w", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("config: write %s: %w", path, err)
	}
	return nil
}
