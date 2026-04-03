# Provider Reasoning Display Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Surface provider-returned raw reasoning text for models that expose it, without adding provider-specific conditionals to the TUI.

**Architecture:** Keep rendering unchanged and fix the provider boundary. `internal/providerpolicy` infers which DeepSeek/Ollama models support thinking, `internal/modelspec` preserves the resulting thinking level, and `internal/agent/model_ollama.go` passes that level to Ollama so SDK responses can populate `schema.Message.ReasoningContent`, which the existing runtime/timeline/TUI path already renders.

**Tech Stack:** Go, CloudWeGo Eino model adapters, standard `go test`.

---

### Task 1: Provider Thinking Capability Inference

**Files:**
- Modify: `internal/providerpolicy/policy.go`
- Test: `internal/providerpolicy/policy_test.go`

- [ ] **Step 1: Add failing tests for DeepSeek/Ollama model inference**

```go
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
```

- [ ] **Step 2: Run policy tests and verify they fail for the new cases**

Run: `go test ./internal/providerpolicy`

Expected: FAIL for the new DeepSeek/Ollama inference tests.

- [ ] **Step 3: Implement minimal model-name inference helpers**

```go
case ProviderDeepSeek:
	if isDeepSeekReasonerModel(id) {
		return ThinkingSupportSupported, []string{"medium"}
	}
case ProviderOllama:
	if isOllamaThinkingModel(id) {
		return ThinkingSupportSupported, []string{"low", "medium", "high"}
	}
```

- [ ] **Step 4: Re-run provider policy tests**

Run: `go test ./internal/providerpolicy`

Expected: PASS.

### Task 2: Ollama Request Thinking Mapping

**Files:**
- Modify: `internal/agent/model_ollama.go`
- Test: `internal/agent/model_test.go`

- [ ] **Step 1: Add a failing test proving `ThinkingLevel` is sent to Ollama**

```go
func TestNewOllamaModelSetsThinkingFromSpec(t *testing.T) {
	// Use an httptest server, call Generate once, decode request JSON,
	// and assert payload["think"] == "medium".
}
```

- [ ] **Step 2: Run agent tests and verify the new case fails**

Run: `go test ./internal/agent`

Expected: FAIL because `newOllamaModel` does not set `cfg.Thinking`.

- [ ] **Step 3: Implement minimal Ollama thinking mapping**

```go
if thinkValue, ok := ollamaThinking(spec.ThinkingLevel); ok {
	cfg.Thinking = thinkValue
}
```

- [ ] **Step 4: Re-run agent tests and the full suite**

Run: `go test ./internal/agent`

Run: `go test ./...`

Expected: PASS.
