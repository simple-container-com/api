package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Supported LLM providers
const (
	ProviderOpenAI    = "openai"
	ProviderOllama    = "ollama"
	ProviderAnthropic = "anthropic"
	ProviderDeepseek  = "deepseek"
	ProviderYandex    = "yandex"
)

// ProviderConfig represents configuration for a specific LLM provider
type ProviderConfig struct {
	APIKey  string `json:"api_key,omitempty"`
	BaseURL string `json:"base_url,omitempty"` // For Ollama or custom endpoints
	Model   string `json:"model,omitempty"`    // Default model for this provider
}

// Config represents the assistant configuration
type Config struct {
	DefaultProvider  string                    `json:"default_provider,omitempty"`   // Last used or preferred provider
	Providers        map[string]ProviderConfig `json:"providers,omitempty"`          // Provider-specific configs
	Preferences      map[string]string         `json:"preferences,omitempty"`        // General preferences
	MaxSavedSessions int                       `json:"max_saved_sessions,omitempty"` // Maximum number of sessions to keep (default: 5)

	// Deprecated fields (kept for backward compatibility)
	OpenAIAPIKey string `json:"openai_api_key,omitempty"`
	LLMProvider  string `json:"llm_provider,omitempty"`
}

// configPath returns the path to the config file
func configPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".sc")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
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
			Providers:   make(map[string]ProviderConfig),
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

	// Initialize maps if nil
	if config.Providers == nil {
		config.Providers = make(map[string]ProviderConfig)
	}
	if config.Preferences == nil {
		config.Preferences = make(map[string]string)
	}

	// Migrate old config format to new format
	if config.OpenAIAPIKey != "" && config.Providers[ProviderOpenAI].APIKey == "" {
		config.Providers[ProviderOpenAI] = ProviderConfig{
			APIKey: config.OpenAIAPIKey,
		}
		config.OpenAIAPIKey = "" // Clear deprecated field
	}
	if config.LLMProvider != "" && config.DefaultProvider == "" {
		config.DefaultProvider = config.LLMProvider
		config.LLMProvider = "" // Clear deprecated field
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
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// SetProviderConfig sets the configuration for a specific provider
func (c *Config) SetProviderConfig(provider string, config ProviderConfig) error {
	if c.Providers == nil {
		c.Providers = make(map[string]ProviderConfig)
	}
	c.Providers[provider] = config
	c.DefaultProvider = provider // Update default to last configured
	return c.Save()
}

// GetProviderConfig gets the configuration for a specific provider
func (c *Config) GetProviderConfig(provider string) (ProviderConfig, bool) {
	if c.Providers == nil {
		return ProviderConfig{}, false
	}
	config, exists := c.Providers[provider]
	return config, exists
}

// DeleteProviderConfig deletes the configuration for a specific provider
func (c *Config) DeleteProviderConfig(provider string) error {
	if c.Providers != nil {
		delete(c.Providers, provider)
	}
	// If we deleted the default provider, clear it
	if c.DefaultProvider == provider {
		c.DefaultProvider = ""
	}
	return c.Save()
}

// HasProviderConfig checks if a provider has configuration
func (c *Config) HasProviderConfig(provider string) bool {
	if c.Providers == nil {
		return false
	}
	config, exists := c.Providers[provider]
	if !exists {
		return false
	}
	// Ollama doesn't require an API key, just needs to exist in config
	if provider == ProviderOllama {
		return true
	}
	// Other providers require an API key
	return config.APIKey != ""
}

// GetDefaultProvider returns the default provider
func (c *Config) GetDefaultProvider() string {
	if c.DefaultProvider != "" {
		return c.DefaultProvider
	}
	// Fallback to openai if available
	if c.HasProviderConfig(ProviderOpenAI) {
		return ProviderOpenAI
	}
	return ""
}

// SetDefaultProvider sets the default provider
func (c *Config) SetDefaultProvider(provider string) error {
	c.DefaultProvider = provider
	return c.Save()
}

// ListProviders returns a list of configured providers
func (c *Config) ListProviders() []string {
	providers := []string{}
	if c.Providers == nil {
		return providers
	}
	for provider := range c.Providers {
		providers = append(providers, provider)
	}
	return providers
}

// IsValidProvider checks if a provider name is valid
func IsValidProvider(provider string) bool {
	provider = strings.ToLower(provider)
	validProviders := []string{
		ProviderOpenAI,
		ProviderOllama,
		ProviderAnthropic,
		ProviderDeepseek,
		ProviderYandex,
	}
	for _, valid := range validProviders {
		if provider == valid {
			return true
		}
	}
	return false
}

// GetProviderDisplayName returns a user-friendly display name for a provider
func GetProviderDisplayName(provider string) string {
	switch strings.ToLower(provider) {
	case ProviderOpenAI:
		return "OpenAI"
	case ProviderOllama:
		return "Ollama"
	case ProviderAnthropic:
		return "Anthropic"
	case ProviderDeepseek:
		return "DeepSeek"
	case ProviderYandex:
		return "Yandex ChatGPT"
	default:
		return provider
	}
}

// Backward compatibility methods
// SetOpenAIAPIKey sets the OpenAI API key (deprecated, use SetProviderConfig)
func (c *Config) SetOpenAIAPIKey(apiKey string) error {
	return c.SetProviderConfig(ProviderOpenAI, ProviderConfig{APIKey: apiKey})
}

// GetOpenAIAPIKey gets the OpenAI API key (deprecated, use GetProviderConfig)
func (c *Config) GetOpenAIAPIKey() string {
	if config, exists := c.GetProviderConfig(ProviderOpenAI); exists {
		return config.APIKey
	}
	return ""
}

// DeleteOpenAIAPIKey deletes the stored OpenAI API key (deprecated, use DeleteProviderConfig)
func (c *Config) DeleteOpenAIAPIKey() error {
	return c.DeleteProviderConfig(ProviderOpenAI)
}

// HasOpenAIAPIKey checks if an OpenAI API key is stored (deprecated, use HasProviderConfig)
func (c *Config) HasOpenAIAPIKey() bool {
	return c.HasProviderConfig(ProviderOpenAI)
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
