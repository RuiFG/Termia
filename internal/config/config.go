package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/BurntSushi/toml"
	"github.com/termia/termia/internal/providerpolicy"
)

// Config represents the complete Termia configuration structure.
type Config struct {
	General GeneralConfig      `toml:"general"`
	LLM     LLMConfig          `toml:"llm"`
	TUI     TUIConfig          `toml:"tui"`
	Sync    SyncConfig         `toml:"sync"`
	Agent   AgentRuntimeConfig `toml:"agent"`
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
	DefaultProvider  string                    `toml:"default_provider"`
	OpenAI           LLMProviderConfig         `toml:"openai"`
	Anthropic        LLMProviderConfig         `toml:"anthropic"`
	Ollama           LLMProviderConfig         `toml:"ollama"`
	DeepSeek         LLMProviderConfig         `toml:"deepseek"`
	OpenAICompatible LLMProviderConfig         `toml:"openai_compatible"`
	CustomProviders  []CustomLLMProviderConfig `toml:"custom_providers"`
}

// LLMProviderConfig contains settings for a specific LLM provider.
type LLMProviderConfig struct {
	APIKey        string `toml:"api_key"`
	Model         string `toml:"model"`
	ThinkingLevel string `toml:"thinking_level"`
	MaxTokens     int    `toml:"max_tokens"`
	BaseURL       string `toml:"base_url"`
}

type CustomLLMProviderConfig struct {
	ID            string `toml:"id"`
	Name          string `toml:"name"`
	Type          string `toml:"type"`
	APIKey        string `toml:"api_key"`
	Model         string `toml:"model"`
	ThinkingLevel string `toml:"thinking_level"`
	MaxTokens     int    `toml:"max_tokens"`
	BaseURL       string `toml:"base_url"`
}

type ProviderMeta struct {
	ID          string
	Kind        string
	DisplayName string
	Config      LLMProviderConfig
	Custom      bool
}

const (
	ProviderOpenAI           = providerpolicy.ProviderOpenAI
	ProviderAnthropic        = providerpolicy.ProviderAnthropic
	ProviderDeepSeek         = providerpolicy.ProviderDeepSeek
	ProviderOllama           = providerpolicy.ProviderOllama
	ProviderOpenAICompatible = providerpolicy.ProviderOpenAICompatible
)

func NormalizeProviderName(provider string) string {
	return providerpolicy.NormalizeProviderName(provider)
}

func ProviderDisplayName(provider string) string {
	return providerpolicy.ProviderDisplayName(provider)
}

func ProviderOrder() []string {
	return providerpolicy.ProviderOrder()
}

func BuiltinProviderOrder() []string {
	return providerpolicy.BuiltinProviderOrder()
}

func (c CustomLLMProviderConfig) ProviderConfig() LLMProviderConfig {
	return LLMProviderConfig{
		APIKey:        c.APIKey,
		Model:         c.Model,
		ThinkingLevel: c.ThinkingLevel,
		MaxTokens:     c.MaxTokens,
		BaseURL:       c.BaseURL,
	}
}

func (c *CustomLLMProviderConfig) SetProviderConfig(cfg LLMProviderConfig) {
	if c == nil {
		return
	}
	c.APIKey = cfg.APIKey
	c.Model = cfg.Model
	c.ThinkingLevel = cfg.ThinkingLevel
	c.MaxTokens = cfg.MaxTokens
	c.BaseURL = cfg.BaseURL
}

func (c *LLMConfig) ProviderMeta(provider string) (ProviderMeta, bool) {
	if c == nil {
		return ProviderMeta{}, false
	}
	provider = NormalizeProviderName(provider)
	switch provider {
	case ProviderOpenAI:
		return ProviderMeta{ID: provider, Kind: ProviderOpenAI, DisplayName: ProviderDisplayName(provider), Config: c.OpenAI}, true
	case ProviderAnthropic:
		return ProviderMeta{ID: provider, Kind: ProviderAnthropic, DisplayName: ProviderDisplayName(provider), Config: c.Anthropic}, true
	case ProviderDeepSeek:
		return ProviderMeta{ID: provider, Kind: ProviderDeepSeek, DisplayName: ProviderDisplayName(provider), Config: c.DeepSeek}, true
	case ProviderOllama:
		return ProviderMeta{ID: provider, Kind: ProviderOllama, DisplayName: ProviderDisplayName(provider), Config: c.Ollama}, true
	case ProviderOpenAICompatible:
		return ProviderMeta{ID: provider, Kind: ProviderOpenAICompatible, DisplayName: ProviderDisplayName(provider), Config: c.OpenAICompatible}, true
	}
	for _, custom := range c.CustomProviders {
		if NormalizeProviderName(custom.ID) != provider {
			continue
		}
		name := strings.TrimSpace(custom.Name)
		if name == "" {
			name = strings.TrimSpace(custom.ID)
		}
		return ProviderMeta{
			ID:          NormalizeProviderName(custom.ID),
			Kind:        NormalizeProviderName(custom.Type),
			DisplayName: name,
			Config:      custom.ProviderConfig(),
			Custom:      true,
		}, true
	}
	return ProviderMeta{}, false
}

func (c *LLMConfig) ProviderDisplayName(provider string) string {
	if meta, ok := c.ProviderMeta(provider); ok {
		return meta.DisplayName
	}
	return ProviderDisplayName(provider)
}

func (c *LLMConfig) ProviderKind(provider string) string {
	if meta, ok := c.ProviderMeta(provider); ok {
		return meta.Kind
	}
	return NormalizeProviderName(provider)
}

func (c *LLMConfig) ManageableProviders() []ProviderMeta {
	if c == nil {
		return nil
	}
	providers := make([]ProviderMeta, 0, len(BuiltinProviderOrder())+len(c.CustomProviders))
	for _, provider := range BuiltinProviderOrder() {
		meta, ok := c.ProviderMeta(provider)
		if ok {
			providers = append(providers, meta)
		}
	}
	custom := make([]ProviderMeta, 0, len(c.CustomProviders))
	for _, provider := range c.CustomProviders {
		meta, ok := c.ProviderMeta(provider.ID)
		if ok {
			custom = append(custom, meta)
		}
	}
	sort.Slice(custom, func(i, j int) bool {
		return strings.ToLower(custom[i].DisplayName) < strings.ToLower(custom[j].DisplayName)
	})
	return append(providers, custom...)
}

func (c *LLMConfig) LegacyOpenAICompatibleActive() bool {
	if c == nil {
		return false
	}
	if NormalizeProviderName(c.DefaultProvider) == ProviderOpenAICompatible {
		return true
	}
	cfg := c.OpenAICompatible
	return strings.TrimSpace(cfg.APIKey) != "" || strings.TrimSpace(cfg.BaseURL) != "" || strings.TrimSpace(cfg.Model) != ""
}

func (c *LLMConfig) ModelProviders() []ProviderMeta {
	providers := c.ManageableProviders()
	if c.LegacyOpenAICompatibleActive() {
		if meta, ok := c.ProviderMeta(ProviderOpenAICompatible); ok {
			providers = append(providers, meta)
		}
	}
	return providers
}

func (c *LLMConfig) ProviderConfigPtr(provider string) (*LLMProviderConfig, bool) {
	if c == nil {
		return nil, false
	}
	switch NormalizeProviderName(provider) {
	case ProviderOpenAI:
		return &c.OpenAI, true
	case ProviderAnthropic:
		return &c.Anthropic, true
	case ProviderDeepSeek:
		return &c.DeepSeek, true
	case ProviderOllama:
		return &c.Ollama, true
	case ProviderOpenAICompatible:
		return &c.OpenAICompatible, true
	default:
		return nil, false
	}
}

func (c *LLMConfig) ProviderConfig(provider string) (LLMProviderConfig, bool) {
	if meta, ok := c.ProviderMeta(provider); ok {
		return meta.Config, true
	}
	return LLMProviderConfig{}, false
}

func (c *LLMConfig) SetProviderConfig(provider string, providerCfg LLMProviderConfig) bool {
	if c == nil {
		return false
	}
	switch NormalizeProviderName(provider) {
	case ProviderOpenAI:
		c.OpenAI = providerCfg
		return true
	case ProviderAnthropic:
		c.Anthropic = providerCfg
		return true
	case ProviderDeepSeek:
		c.DeepSeek = providerCfg
		return true
	case ProviderOllama:
		c.Ollama = providerCfg
		return true
	case ProviderOpenAICompatible:
		c.OpenAICompatible = providerCfg
		return true
	}
	for i := range c.CustomProviders {
		if NormalizeProviderName(c.CustomProviders[i].ID) != NormalizeProviderName(provider) {
			continue
		}
		c.CustomProviders[i].SetProviderConfig(providerCfg)
		return true
	}
	return false
}

func (c *LLMConfig) AddCustomProvider(name, providerType string, providerCfg LLMProviderConfig) (CustomLLMProviderConfig, error) {
	if c == nil {
		return CustomLLMProviderConfig{}, fmt.Errorf("llm config is nil")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return CustomLLMProviderConfig{}, fmt.Errorf("provider name is required")
	}
	customID := nextCustomProviderID(name, c.CustomProviders)
	for _, existing := range c.CustomProviders {
		if strings.EqualFold(strings.TrimSpace(existing.Name), name) || NormalizeProviderName(existing.ID) == customID {
			return CustomLLMProviderConfig{}, fmt.Errorf("provider %q already exists", name)
		}
	}
	providerType = NormalizeProviderName(providerType)
	if providerType != ProviderOpenAICompatible {
		return CustomLLMProviderConfig{}, fmt.Errorf("unsupported custom provider type %q", providerType)
	}
	custom := CustomLLMProviderConfig{
		ID:   customID,
		Name: name,
		Type: providerType,
	}
	custom.SetProviderConfig(providerCfg)
	c.CustomProviders = append(c.CustomProviders, custom)
	return custom, nil
}

func (c *LLMConfig) RemoveCustomProvider(provider string) bool {
	if c == nil {
		return false
	}
	provider = NormalizeProviderName(provider)
	for i := range c.CustomProviders {
		if NormalizeProviderName(c.CustomProviders[i].ID) != provider {
			continue
		}
		c.CustomProviders = append(c.CustomProviders[:i], c.CustomProviders[i+1:]...)
		return true
	}
	return false
}

func (c LLMProviderConfig) ResolvedAPIKey() string {
	return strings.TrimSpace(c.APIKey)
}

func nextCustomProviderID(name string, providers []CustomLLMProviderConfig) string {
	base := slugifyProviderName(name)
	if base == "" {
		base = "provider"
	}
	id := "custom:" + base
	used := make(map[string]struct{}, len(providers))
	for _, provider := range providers {
		used[NormalizeProviderName(provider.ID)] = struct{}{}
	}
	if _, ok := used[id]; !ok {
		return id
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", id, i)
		if _, ok := used[candidate]; !ok {
			return candidate
		}
	}
}

func slugifyProviderName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case lastDash:
			continue
		default:
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
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

// AgentRuntimeConfig contains runtime settings for assistant/team execution.
type AgentRuntimeConfig struct {
	DefaultMode                string `toml:"default_mode"`
	DefaultTeam                string `toml:"default_team"`
	TeamsDir                   string `toml:"teams_dir"`
	RequireCommandConfirmation bool   `toml:"require_command_confirmation"`
	DefaultStreamMaxLines      int    `toml:"default_stream_max_lines"`
	DefaultStreamTimeoutMs     int    `toml:"default_stream_timeout_ms"`
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
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create config directory for %s: %w", path, err)
	}
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
