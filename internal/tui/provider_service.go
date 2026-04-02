package tui

import (
	"fmt"
	"strings"

	"github.com/termia/termia/internal/config"
	"github.com/termia/termia/internal/llm"
	"github.com/termia/termia/internal/providerpolicy"
)

type providerService struct {
	cfg        *config.Config
	saveConfig func(*config.Config) error
}

func newProviderService(cfg *config.Config, saveConfig func(*config.Config) error) providerService {
	return providerService{cfg: cfg, saveConfig: saveConfig}
}

func (s providerService) ManageableProviders() []config.ProviderMeta {
	if s.cfg == nil {
		return nil
	}
	return s.cfg.LLM.ManageableProviders()
}

func (s providerService) ModelProviders() []config.ProviderMeta {
	if s.cfg == nil {
		return nil
	}
	return s.cfg.LLM.ModelProviders()
}

func (s providerService) ProviderMeta(provider string) (config.ProviderMeta, bool) {
	if s.cfg == nil {
		return config.ProviderMeta{}, false
	}
	return s.cfg.LLM.ProviderMeta(provider)
}

func (s providerService) ProviderDisplayName(provider string) string {
	if s.cfg == nil {
		return providerpolicy.ProviderDisplayName(provider)
	}
	return s.cfg.LLM.ProviderDisplayName(provider)
}

func (s providerService) ProviderKind(provider string) string {
	if s.cfg == nil {
		return providerpolicy.NormalizeProviderName(provider)
	}
	return s.cfg.LLM.ProviderKind(provider)
}

func (s providerService) ProviderConfig(provider string) (config.LLMProviderConfig, bool) {
	if s.cfg == nil {
		return config.LLMProviderConfig{}, false
	}
	return s.cfg.LLM.ProviderConfig(provider)
}

func (s providerService) ValidateProvider(meta config.ProviderMeta) error {
	return llm.ValidateProviderConfig(meta.Kind, meta.Config)
}

func (s providerService) ValidateModelCatalog(meta config.ProviderMeta) error {
	return llm.ValidateModelCatalogConfig(meta)
}

func (s providerService) ConfigFields(provider string) []llm.ProviderConfigFieldSpec {
	return llm.ConfigFields(provider)
}

func (s providerService) ProviderFieldRawValue(provider string, field llm.ProviderConfigField) string {
	providerCfg, ok := s.ProviderConfig(provider)
	if !ok {
		return ""
	}
	switch field {
	case llm.ProviderFieldAPIKey:
		return strings.TrimSpace(providerCfg.APIKey)
	case llm.ProviderFieldBaseURL:
		return strings.TrimSpace(providerCfg.BaseURL)
	default:
		return ""
	}
}

func (s providerService) ProviderFieldDisplayValue(provider string, field llm.ProviderConfigField) string {
	providerCfg, ok := s.ProviderConfig(provider)
	if !ok {
		return "(not set)"
	}
	providerKind := s.ProviderKind(provider)
	switch field {
	case llm.ProviderFieldAPIKey:
		return maskSecret(providerCfg.ResolvedAPIKey())
	case llm.ProviderFieldBaseURL:
		value := strings.TrimSpace(providerCfg.BaseURL)
		if value == "" {
			value = llm.EffectiveBaseURL(providerKind, providerCfg)
		}
		if value == "" {
			return "(not set)"
		}
		return value
	default:
		return "(not set)"
	}
}

func (s providerService) UpdateProviderField(provider string, field llm.ProviderConfigField, value string) error {
	if s.cfg == nil {
		return fmt.Errorf("config is nil")
	}
	provider = providerpolicy.NormalizeProviderName(provider)
	providerCfg, ok := s.cfg.LLM.ProviderConfig(provider)
	if !ok {
		return fmt.Errorf("unsupported provider")
	}
	switch field {
	case llm.ProviderFieldAPIKey:
		providerCfg.APIKey = strings.TrimSpace(value)
	case llm.ProviderFieldBaseURL:
		providerCfg.BaseURL = strings.TrimSpace(value)
	default:
		return fmt.Errorf("unsupported field")
	}
	if !s.cfg.LLM.SetProviderConfig(provider, providerCfg) {
		return fmt.Errorf("unsupported provider")
	}
	return s.save()
}

func (s providerService) CreateCustomProvider(name, apiKey, baseURL string) (config.CustomLLMProviderConfig, error) {
	if s.cfg == nil {
		return config.CustomLLMProviderConfig{}, fmt.Errorf("config is nil")
	}
	custom, err := s.cfg.LLM.AddCustomProvider(name, config.ProviderOpenAICompatible, config.LLMProviderConfig{
		APIKey:    strings.TrimSpace(apiKey),
		Model:     "",
		MaxTokens: 2000,
		BaseURL:   strings.TrimSpace(baseURL),
	})
	if err != nil {
		return config.CustomLLMProviderConfig{}, err
	}
	if err := s.save(); err != nil {
		return config.CustomLLMProviderConfig{}, err
	}
	return custom, nil
}

func (s providerService) ClearProviderField(provider string, field llm.ProviderConfigField) error {
	return s.UpdateProviderField(provider, field, "")
}

func (s providerService) DeleteCustomProvider(provider string) (config.ProviderMeta, error) {
	if s.cfg == nil {
		return config.ProviderMeta{}, fmt.Errorf("config is nil")
	}
	provider = providerpolicy.NormalizeProviderName(provider)
	meta, ok := s.cfg.LLM.ProviderMeta(provider)
	if !ok || !meta.Custom {
		return config.ProviderMeta{}, fmt.Errorf("only custom providers can be deleted")
	}
	if !s.cfg.LLM.RemoveCustomProvider(provider) {
		return config.ProviderMeta{}, fmt.Errorf("provider not found")
	}
	if providerpolicy.NormalizeProviderName(s.cfg.LLM.DefaultProvider) == provider {
		s.cfg.LLM.DefaultProvider = config.ProviderOpenAI
	}
	if err := s.save(); err != nil {
		return config.ProviderMeta{}, err
	}
	return meta, nil
}

func (s providerService) SelectModel(provider string, modelID string) error {
	if s.cfg == nil {
		return fmt.Errorf("config is nil")
	}
	provider = providerpolicy.NormalizeProviderName(provider)
	providerCfg, ok := s.cfg.LLM.ProviderConfig(provider)
	if !ok {
		return fmt.Errorf("unsupported provider")
	}
	providerCfg.Model = strings.TrimSpace(modelID)
	if !s.cfg.LLM.SetProviderConfig(provider, providerCfg) {
		return fmt.Errorf("unsupported provider")
	}
	s.cfg.LLM.DefaultProvider = provider
	return s.save()
}

func (s providerService) CurrentModelDescriptor(providerModels map[string][]llm.ModelDescriptor) (llm.ModelDescriptor, bool) {
	if s.cfg == nil {
		return llm.ModelDescriptor{}, false
	}
	provider := providerpolicy.NormalizeProviderName(s.cfg.LLM.DefaultProvider)
	providerCfg, ok := s.cfg.LLM.ProviderConfig(provider)
	if !ok {
		return llm.ModelDescriptor{}, false
	}
	modelID := strings.TrimSpace(providerCfg.Model)
	if modelID == "" {
		return llm.ModelDescriptor{}, false
	}
	for _, model := range providerModels[provider] {
		if strings.EqualFold(strings.TrimSpace(model.ID), modelID) {
			return model, true
		}
	}
	return llm.ModelDescriptor{}, false
}

func (s providerService) CurrentConfiguredThinkingLevel() string {
	if s.cfg == nil {
		return ""
	}
	provider := providerpolicy.NormalizeProviderName(s.cfg.LLM.DefaultProvider)
	providerCfg, ok := s.cfg.LLM.ProviderConfig(provider)
	if !ok {
		return ""
	}
	return providerpolicy.NormalizeThinkingLevel(providerCfg.ThinkingLevel)
}

func (s providerService) PersistCurrentThinkingLevel(level string) error {
	if s.cfg == nil {
		return fmt.Errorf("config is nil")
	}
	provider := providerpolicy.NormalizeProviderName(s.cfg.LLM.DefaultProvider)
	providerCfg, ok := s.cfg.LLM.ProviderConfig(provider)
	if !ok {
		return fmt.Errorf("unsupported provider")
	}
	value := providerpolicy.NormalizeThinkingLevel(level)
	if strings.TrimSpace(providerCfg.ThinkingLevel) == value {
		return nil
	}
	providerCfg.ThinkingLevel = value
	if !s.cfg.LLM.SetProviderConfig(provider, providerCfg) {
		return fmt.Errorf("unsupported provider")
	}
	return s.save()
}

func (s providerService) save() error {
	if s.saveConfig == nil || s.cfg == nil {
		return nil
	}
	return s.saveConfig(s.cfg)
}
