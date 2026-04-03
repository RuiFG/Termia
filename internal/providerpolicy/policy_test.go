package providerpolicy

import "testing"

func TestNormalizeProviderNameMapsClaudeAlias(t *testing.T) {
	if got := NormalizeProviderName("claude"); got != ProviderAnthropic {
		t.Fatalf("expected claude alias to normalize to anthropic, got %q", got)
	}
}

func TestUsesNativeOpenAIResponsesRecognizesOpenAICompatibleOpenAIHost(t *testing.T) {
	if !UsesNativeOpenAIResponses(ProviderOpenAICompatible, "https://api.openai.com/v1") {
		t.Fatal("expected openai-compatible api.openai.com to use native responses")
	}
	if UsesNativeOpenAIResponses(ProviderOpenAICompatible, "https://example.com/v1") {
		t.Fatal("expected non-openai host to stay on compatible chat path")
	}
}

func TestThinkingLevelsForModelRestrictsChatLatestToMedium(t *testing.T) {
	levels := ThinkingLevelsForModel(ProviderOpenAI, "gpt-5.2-chat-latest")
	if len(levels) != 1 || levels[0] != "medium" {
		t.Fatalf("expected chat-latest to only support medium, got %#v", levels)
	}
}

func TestThinkingLevelsForModelInfersDeepSeekReasoner(t *testing.T) {
	levels := ThinkingLevelsForModel(ProviderDeepSeek, "deepseek-reasoner")
	if len(levels) != 1 || levels[0] != "medium" {
		t.Fatalf("expected deepseek-reasoner to expose medium thinking, got %#v", levels)
	}
}

func TestThinkingLevelsForModelInfersOllamaThinkingModels(t *testing.T) {
	levels := ThinkingLevelsForModel(ProviderOllama, "qwen3:8b")
	if len(levels) != 3 || levels[0] != "low" || levels[1] != "medium" || levels[2] != "high" {
		t.Fatalf("expected qwen3 ollama thinking levels, got %#v", levels)
	}
}

func TestIsOpenAIResponsesOnlyModelRecognizesCodex(t *testing.T) {
	if !IsOpenAIResponsesOnlyModel("gpt-5.1-codex") {
		t.Fatal("expected codex to require responses API")
	}
	if IsOpenAIResponsesOnlyModel("gpt-5.1") {
		t.Fatal("expected non-codex gpt-5.1 to stay non-responses-only")
	}
}
