package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config represents the assistant configuration
type Config struct {
	OpenAIAPIKey string            `json:"openai_api_key,omitempty"`
	LLMProvider  string            `json:"llm_provider,omitempty"`
	Preferences  map[string]string `json:"preferences,omitempty"`
}

// configPath returns the path to the config file
func configPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".sc")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create config directory: %w", err)
	}

	return filepath.Join(configDir, "assistant-config.json"), nil
}

// Load loads the configuration from disk
func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	// If config doesn't exist, return empty config
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return &Config{
			Preferences: make(map[string]string),
		}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if config.Preferences == nil {
		config.Preferences = make(map[string]string)
	}

	return &config, nil
}

// Save saves the configuration to disk
func (c *Config) Save() error {
	path, err := configPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write with restricted permissions (0600 = rw-------)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// SetOpenAIAPIKey sets the OpenAI API key
func (c *Config) SetOpenAIAPIKey(apiKey string) error {
	c.OpenAIAPIKey = apiKey
	return c.Save()
}

// GetOpenAIAPIKey gets the OpenAI API key
func (c *Config) GetOpenAIAPIKey() string {
	return c.OpenAIAPIKey
}

// DeleteOpenAIAPIKey deletes the stored OpenAI API key
func (c *Config) DeleteOpenAIAPIKey() error {
	c.OpenAIAPIKey = ""
	return c.Save()
}

// HasOpenAIAPIKey checks if an OpenAI API key is stored
func (c *Config) HasOpenAIAPIKey() bool {
	return c.OpenAIAPIKey != ""
}

// SetPreference sets a preference value
func (c *Config) SetPreference(key, value string) error {
	if c.Preferences == nil {
		c.Preferences = make(map[string]string)
	}
	c.Preferences[key] = value
	return c.Save()
}

// GetPreference gets a preference value
func (c *Config) GetPreference(key string) (string, bool) {
	if c.Preferences == nil {
		return "", false
	}
	val, ok := c.Preferences[key]
	return val, ok
}

// ConfigPath returns the path to the config file for display purposes
func ConfigPath() (string, error) {
	return configPath()
}
