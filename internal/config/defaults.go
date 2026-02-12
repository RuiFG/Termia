package config

// DefaultConfig returns a Config with sensible default values.
// These defaults match the values in scripts/config.toml.
func DefaultConfig() *Config {
	return &Config{
		General: GeneralConfig{
			StoragePath:           "~/.termia",
			MaxTranscriptAgeDays:  90,
			MaxDBSizeMB:           1000,
			RecordOutputs:         true,
			RecordEnvVars:         false,
			IgnorePatterns: []string{
				"^pass ",
				"^export .*SECRET",
			},
		},
		LLM: LLMConfig{
			DefaultProvider: "openai",
			OpenAI: LLMProviderConfig{
				APIKeyEnv:  "OPENAI_API_KEY",
				Model:      "gpt-4o",
				MaxTokens:  2000,
				BaseURL:    "",
			},
			Anthropic: LLMProviderConfig{
				APIKeyEnv: "ANTHROPIC_API_KEY",
				Model:     "claude-3-7-sonnet-20250219",
				MaxTokens: 2000,
				BaseURL:   "",
			},
			Ollama: LLMProviderConfig{
				APIKeyEnv: "",
				Model:     "llama3.2:latest",
				MaxTokens: 0,
				BaseURL:   "http://localhost:11434/v1",
			},
			DeepSeek: LLMProviderConfig{
				APIKeyEnv: "DEEPSEEK_API_KEY",
				Model:     "deepseek-chat",
				MaxTokens: 0,
				BaseURL:   "https://api.deepseek.com",
			},
		},
		TUI: TUIConfig{
			Theme:           "default",
			ShowExitCodes:   true,
			HighlightErrors: true,
			Navigation:      "vim",
		},
		Sync: SyncConfig{
			Enabled:              false,
			Endpoint:             "https://api.termia.io",
			AutoSyncIntervalMin:  15,
			EncryptLocal:         true,
		},
	}
}
