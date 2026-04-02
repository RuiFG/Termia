package llm

import (
	"testing"

	"github.com/termia/termia/internal/config"
)

func TestThinkingLevelsForModelInfersOpenAIReasoningModels(t *testing.T) {
	levels := ThinkingLevelsForModel(config.ProviderOpenAI, "gpt-5")
	if len(levels) != 3 || levels[0] != "low" || levels[1] != "medium" || levels[2] != "high" {
		t.Fatalf("expected openai reasoning levels, got %#v", levels)
	}
}

func TestThinkingLevelsForModelInfersOpenAICompatibleReasoningModels(t *testing.T) {
	levels := ThinkingLevelsForModel(config.ProviderOpenAICompatible, "openai/gpt-5")
	if len(levels) != 3 || levels[0] != "low" || levels[1] != "medium" || levels[2] != "high" {
		t.Fatalf("expected openai-compatible reasoning levels, got %#v", levels)
	}
}

func TestThinkingLevelsForModelDoesNotInferGPT4oAsReasoningModel(t *testing.T) {
	levels := ThinkingLevelsForModel(config.ProviderOpenAI, "gpt-4o")
	if len(levels) != 0 {
		t.Fatalf("expected no reasoning levels for gpt-4o, got %#v", levels)
	}
}

func TestThinkingLevelsForModelRestrictsChatLatestToMedium(t *testing.T) {
	levels := ThinkingLevelsForModel(config.ProviderOpenAI, "gpt-5.1-chat-latest")
	if len(levels) != 1 || levels[0] != "medium" {
		t.Fatalf("expected only medium for chat-latest, got %#v", levels)
	}
}

func TestSupportsThinkingLevelRejectsUnsupportedChatLatestLevels(t *testing.T) {
	if SupportsThinkingLevel(config.ProviderOpenAI, "gpt-5.1-chat-latest", "high") {
		t.Fatal("expected high to be rejected for gpt-5.1-chat-latest")
	}
	if !SupportsThinkingLevel(config.ProviderOpenAI, "gpt-5.1-chat-latest", "medium") {
		t.Fatal("expected medium to be accepted for gpt-5.1-chat-latest")
	}
}

func TestThinkingLevelsForModelInfersAnthropicThinkingModels(t *testing.T) {
	levels := ThinkingLevelsForModel(config.ProviderAnthropic, "claude-3-7-sonnet-20250219")
	if len(levels) != 3 || levels[0] != "low" || levels[1] != "medium" || levels[2] != "high" {
		t.Fatalf("expected anthropic thinking levels, got %#v", levels)
	}
}

func TestDefaultThinkingLevelPrefersMedium(t *testing.T) {
	level := DefaultThinkingLevel(config.ProviderOpenAI, "gpt-5")
	if level != "medium" {
		t.Fatalf("expected medium default, got %q", level)
	}
}

func TestNewDescriptorUsesInferenceWhenProviderMetadataIsSilent(t *testing.T) {
	descriptor := newDescriptor(config.ProviderOpenAI, "gpt-5", "GPT-5", `{}`)
	if descriptor.ThinkingSupport != ThinkingSupportSupported {
		t.Fatalf("expected inferred thinking support, got %v", descriptor.ThinkingSupport)
	}
	if len(descriptor.ThinkingLevels) != 3 || descriptor.ThinkingLevels[1] != "medium" {
		t.Fatalf("expected inferred thinking levels, got %#v", descriptor.ThinkingLevels)
	}
}
