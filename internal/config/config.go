package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// Config represents the complete Termia configuration structure.
type Config struct {
	General GeneralConfig `toml:"general"`
	LLM     LLMConfig     `toml:"llm"`
	TUI     TUIConfig     `toml:"tui"`
	Sync    SyncConfig    `toml:"sync"`
}

// GeneralConfig contains general application settings.
type GeneralConfig struct {
	StoragePath          string   `toml:"storage_path"`
	MaxTranscriptAgeDays int      `toml:"max_transcript_age_days"`
	MaxDBSizeMB          int      `toml:"max_db_size_mb"`
	RecordOutputs        bool     `toml:"record_outputs"`
	RecordEnvVars        bool     `toml:"record_env_vars"`
	IgnorePatterns       []string `toml:"ignore_patterns"`
}

// LLMConfig contains LLM provider configuration.
type LLMConfig struct {
	DefaultProvider string            `toml:"default_provider"`
	OpenAI          LLMProviderConfig `toml:"openai"`
	Anthropic       LLMProviderConfig `toml:"anthropic"`
	Ollama          LLMProviderConfig `toml:"ollama"`
	DeepSeek        LLMProviderConfig `toml:"deepseek"`
}

// LLMProviderConfig contains settings for a specific LLM provider.
type LLMProviderConfig struct {
	APIKeyEnv string `toml:"api_key_env"`
	Model     string `toml:"model"`
	MaxTokens int    `toml:"max_tokens"`
	BaseURL   string `toml:"base_url"`
}

// TUIConfig contains terminal UI settings.
type TUIConfig struct {
	Theme           string `toml:"theme"`
	ShowExitCodes   bool   `toml:"show_exit_codes"`
	HighlightErrors bool   `toml:"highlight_errors"`
	Navigation      string `toml:"navigation"`
}

// SyncConfig contains cloud sync settings.
type SyncConfig struct {
	Enabled             bool   `toml:"enabled"`
	Endpoint            string `toml:"endpoint"`
	AutoSyncIntervalMin int    `toml:"auto_sync_interval_min"`
	EncryptLocal        bool   `toml:"encrypt_local"`
}

// Load reads and parses a TOML configuration file from the given path.
// Returns an error if the file cannot be read or parsed.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", path, err)
	}

	return &cfg, nil
}

// LoadOrDefault attempts to load the configuration from ConfigPath().
// If the file doesn't exist or cannot be loaded, returns DefaultConfig() instead.
// Only returns an error if the file exists but is malformed.
func LoadOrDefault() (*Config, error) {
	path := ConfigPath()

	// Check if config file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return DefaultConfig(), nil
	}

	// Try to load the config
	cfg, err := Load(path)
	if err != nil {
		return nil, fmt.Errorf("config file exists but failed to load: %w", err)
	}

	return cfg, nil
}

// Save writes the configuration to a TOML file at the given path.
// Returns an error if the file cannot be written.
func Save(cfg *Config, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create config file %s: %w", path, err)
	}
	defer f.Close()

	encoder := toml.NewEncoder(f)
	if err := encoder.Encode(cfg); err != nil {
		return fmt.Errorf("failed to encode config to TOML: %w", err)
	}

	return nil
}
