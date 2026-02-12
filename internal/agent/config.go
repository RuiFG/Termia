package agent

import (
	"fmt"
	"os"
	"strings"

	"github.com/termia/termia/internal/config"
)

const DefaultSystemPrompt = `You are Termia, a transparent terminal assistant. Provide clear, concise analysis of terminal activity and tasks. Use the provided command history as ground truth, ask clarifying questions only when essential, and avoid inventing outputs or files that do not exist. When suggesting shell commands, prefer using the command tool so execution is approved by the user.`

// AgentConfig contains runtime configuration for the LLM agent.
type AgentConfig struct {
	Provider     string
	Model        string
	APIKey       string
	BaseURL      string
	MaxTokens    int
	Temperature  float64
	SystemPrompt string
}

// NewAgentConfigFromConfig builds an AgentConfig from the application's LLM config.
func NewAgentConfigFromConfig(llmCfg *config.LLMConfig) (*AgentConfig, error) {
	if llmCfg == nil {
		return nil, fmt.Errorf("llm config is nil")
	}

	providerCfg, providerName, err := resolveProviderConfig(llmCfg)
	if err != nil {
		return nil, fmt.Errorf("resolve provider: %w", err)
	}

	return newAgentConfigFromProvider(llmCfg, providerName, providerCfg)
}

// NewAgentConfigFromProvider builds an AgentConfig from a specific provider name.
func NewAgentConfigFromProvider(llmCfg *config.LLMConfig, providerName string) (*AgentConfig, error) {
	if llmCfg == nil {
		return nil, fmt.Errorf("llm config is nil")
	}
	providerCfg, resolvedName, err := resolveProviderConfigByName(llmCfg, providerName)
	if err != nil {
		return nil, fmt.Errorf("resolve provider: %w", err)
	}
	return newAgentConfigFromProvider(llmCfg, resolvedName, providerCfg)
}

func newAgentConfigFromProvider(_ *config.LLMConfig, providerName string, providerCfg *config.LLMProviderConfig) (*AgentConfig, error) {
	if providerCfg == nil {
		return nil, fmt.Errorf("provider config is nil")
	}

	apiKey := ""
	if providerName != "ollama" || providerCfg.APIKeyEnv != "" {
		resolved, err := resolveAPIKey(providerCfg.APIKeyEnv)
		if err != nil {
			return nil, fmt.Errorf("resolve api key: %w", err)
		}
		apiKey = resolved
	}

	if strings.TrimSpace(providerCfg.Model) == "" {
		return nil, fmt.Errorf("model is required for provider %s", providerName)
	}

	return &AgentConfig{
		Provider:     providerName,
		Model:        providerCfg.Model,
		APIKey:       apiKey,
		BaseURL:      providerCfg.BaseURL,
		MaxTokens:    providerCfg.MaxTokens,
		Temperature:  0.2,
		SystemPrompt: DefaultSystemPrompt,
	}, nil
}

// resolveProviderConfig selects the provider configuration based on the default provider.
func resolveProviderConfig(llmCfg *config.LLMConfig) (*config.LLMProviderConfig, string, error) {
	if llmCfg == nil {
		return nil, "", fmt.Errorf("llm config is nil")
	}

	provider := strings.ToLower(strings.TrimSpace(llmCfg.DefaultProvider))
	return resolveProviderConfigByName(llmCfg, provider)
}

func resolveProviderConfigByName(llmCfg *config.LLMConfig, provider string) (*config.LLMProviderConfig, string, error) {
	switch provider {
	case "openai":
		return &llmCfg.OpenAI, provider, nil
	case "anthropic":
		return &llmCfg.Anthropic, provider, nil
	case "ollama":
		return &llmCfg.Ollama, provider, nil
	case "deepseek":
		return &llmCfg.DeepSeek, provider, nil
	default:
		return nil, "", fmt.Errorf("unsupported provider: %s", provider)
	}
}

// resolveAPIKey reads the API key from the environment variable.
func resolveAPIKey(envVar string) (string, error) {
	if strings.TrimSpace(envVar) == "" {
		return "", fmt.Errorf("api key environment variable is not configured")
	}

	apiKey := strings.TrimSpace(os.Getenv(envVar))
	if apiKey == "" {
		return "", fmt.Errorf("api key not found in %s", envVar)
	}

	return apiKey, nil
}
