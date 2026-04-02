package config

// DefaultConfig returns a Config with sensible default values.
// These defaults match the values in scripts/config.toml.
func DefaultConfig() *Config {
	return &Config{
		General: GeneralConfig{
			StoragePath:          "~/.termia",
			MaxTranscriptAgeDays: 90,
			MaxDBSizeMB:          1000,
			RecordOutputs:        true,
			RecordEnvVars:        false,
			IgnorePatterns: []string{
				"^pass ",
				"^export .*SECRET",
			},
		},
		LLM: LLMConfig{
			DefaultProvider: "openai",
			OpenAI: LLMProviderConfig{
				APIKey:        "",
				Model:         "gpt-4o",
				ThinkingLevel: "",
				MaxTokens:     2000,
				BaseURL:       "",
			},
			Anthropic: LLMProviderConfig{
				APIKey:        "",
				Model:         "claude-3-7-sonnet-20250219",
				ThinkingLevel: "",
				MaxTokens:     2000,
				BaseURL:       "",
			},
			Ollama: LLMProviderConfig{
				APIKey:        "",
				Model:         "llama3.2:latest",
				ThinkingLevel: "",
				MaxTokens:     0,
				BaseURL:       "http://localhost:11434",
			},
			DeepSeek: LLMProviderConfig{
				APIKey:        "",
				Model:         "deepseek-chat",
				ThinkingLevel: "",
				MaxTokens:     0,
				BaseURL:       "https://api.deepseek.com",
			},
			OpenAICompatible: LLMProviderConfig{
				APIKey:        "",
				Model:         "",
				ThinkingLevel: "",
				MaxTokens:     2000,
				BaseURL:       "",
			},
		},
		TUI: TUIConfig{
			Theme:           "default",
			ShowExitCodes:   true,
			HighlightErrors: true,
			Navigation:      "vim",
		},
		Sync: SyncConfig{
			Enabled:             false,
			Endpoint:            "https://api.termia.io",
			AutoSyncIntervalMin: 15,
			EncryptLocal:        true,
		},
		Agent: AgentRuntimeConfig{
			DefaultMode:                "assistant",
			DefaultTeam:                "",
			TeamsDir:                   "",
			RequireCommandConfirmation: true,
			DefaultStreamMaxLines:      120,
			DefaultStreamTimeoutMs:     3000,
		},
	}
}
