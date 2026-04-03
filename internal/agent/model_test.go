package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/schema"
	"github.com/termia/termia/internal/config"
)

func TestOpenAIReasoningEffortForModelSkipsUnsupportedModels(t *testing.T) {
	if effort, ok := openAIReasoningEffortForModel(config.ProviderOpenAI, "gpt-4o", "high"); ok {
		t.Fatalf("expected gpt-4o to skip reasoning effort, got %q", effort)
	}
}

func TestOpenAIReasoningEffortForModelAllowsOpenAIReasoningModels(t *testing.T) {
	effort, ok := openAIReasoningEffortForModel(config.ProviderOpenAI, "gpt-5", "medium")
	if !ok {
		t.Fatal("expected gpt-5 to accept reasoning effort")
	}
	if effort != openai.ReasoningEffortLevelMedium {
		t.Fatalf("expected medium effort, got %q", effort)
	}
}

func TestOpenAIReasoningEffortForModelAllowsOpenAICompatibleReasoningModels(t *testing.T) {
	effort, ok := openAIReasoningEffortForModel(config.ProviderOpenAICompatible, "openai/gpt-5", "high")
	if !ok {
		t.Fatal("expected openai-compatible gpt-5 to accept reasoning effort")
	}
	if effort != openai.ReasoningEffortLevelHigh {
		t.Fatalf("expected high effort, got %q", effort)
	}
}
func TestOpenAIReasoningEffortForModelRestrictsChatLatestToMedium(t *testing.T) {
	if effort, ok := openAIReasoningEffortForModel(config.ProviderOpenAI, "gpt-5.1-chat-latest", "high"); ok {
		t.Fatalf("expected chat-latest high to be rejected, got %q", effort)
	}
	effort, ok := openAIReasoningEffortForModel(config.ProviderOpenAI, "gpt-5.1-chat-latest", "medium")
	if !ok || effort != openai.ReasoningEffortLevelMedium {
		t.Fatalf("expected chat-latest medium effort, got %q ok=%v", effort, ok)
	}
}

func TestNewOpenAIModelUsesAgenticForNativeOpenAI(t *testing.T) {
	got, err := newOpenAIModel(context.Background(), ModelSpec{
		Provider: config.ProviderOpenAI,
		Model:    "gpt-5.1-codex",
	})
	if err != nil {
		t.Fatalf("expected native openai model to use agentic responses, got %v", err)
	}
	if _, ok := got.(*openAIAgenticChatModel); !ok {
		t.Fatalf("expected openAIAgenticChatModel, got %T", got)
	}
}

func TestEffectiveModelBaseURLDefaultsOpenAI(t *testing.T) {
	got := effectiveModelBaseURL(ModelSpec{Provider: config.ProviderOpenAI})
	if got != "https://api.openai.com/v1" {
		t.Fatalf("expected openai default base url, got %q", got)
	}
}

func TestNewOllamaModelSetsThinkingFromSpec(t *testing.T) {
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintln(w, `{"message":{"role":"assistant","content":"done","thinking":"plan"},"done":true}`)
	}))
	defer server.Close()

	got, err := newOllamaModel(context.Background(), ModelSpec{
		Provider:      config.ProviderOllama,
		Model:         "qwen3:8b",
		BaseURL:       server.URL,
		ThinkingLevel: "medium",
	})
	if err != nil {
		t.Fatalf("new ollama model: %v", err)
	}

	msg, err := got.Generate(context.Background(), []*schema.Message{{
		Role:    schema.User,
		Content: "hello",
	}})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if msg == nil || msg.ReasoningContent != "plan" {
		t.Fatalf("expected ollama reasoning content to round-trip, got %#v", msg)
	}
	if payload["think"] != "medium" {
		t.Fatalf("expected ollama think=medium, got %#v", payload["think"])
	}
}
