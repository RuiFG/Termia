# OpenAI Reasoning Summary Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Display OpenAI Responses API reasoning summaries whenever the selected OpenAI or OpenAI-compatible model/provider returns them.

**Architecture:** Keep TUI/runtime unchanged and validate the provider adapter boundary. `newOpenAIAgenticModel` should request reasoning summaries for supported models, `agenticMessageToMessage` should convert reasoning blocks into `schema.Message` reasoning parts, and the existing `assistantContentEvents` path should emit `RuntimeEventReasoning`.

**Tech Stack:** Go, CloudWeGo Eino `agenticopenai`, standard `go test`.

---

### Task 1: Lock OpenAI Reasoning Summary Conversion

**Files:**
- Test: `internal/agent/openai_agentic_messages_out_test.go`
- Test: `internal/agent/model_test.go`
- Modify if needed: `internal/agent/model_openai.go`

- [ ] **Step 1: Add tests for reasoning-block conversion and request config**

```go
func TestAgenticMessageToMessageConvertsReasoningBlocks(t *testing.T) {
	msg, err := agenticMessageToMessage(&schema.AgenticMessage{
		Role: schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{
			schema.NewContentBlock(&schema.Reasoning{Text: "summary"}),
		},
	})
	if err != nil {
		t.Fatalf("agenticMessageToMessage: %v", err)
	}
	events := assistantContentEvents(msg)
	if len(events) != 1 || events[0].Kind != RuntimeEventReasoning || events[0].Text != "summary" {
		t.Fatalf("expected reasoning summary event, got %#v", events)
	}
}
```

- [ ] **Step 2: Run targeted tests and inspect whether they fail because of a real gap**

Run: `go test ./internal/agent`

Expected: PASS if conversion already works; otherwise FAIL on the new OpenAI summary test.

- [ ] **Step 3: If the request config path is missing summary setup, fix only `model_openai.go`**

```go
if reasoning, ok := openAIResponsesReasoningForModel(spec.Provider, spec.Model, spec.ThinkingLevel); ok {
	cfg.Reasoning = reasoning
}
```

- [ ] **Step 4: Re-run agent tests and the full suite**

Run: `go test ./internal/agent`

Run: `go test ./...`

Expected: PASS.
