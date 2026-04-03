package agent

import (
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestAgenticMessageToMessageCopiesReasoningSummaryToReasoningContent(t *testing.T) {
	msg, err := agenticMessageToMessage(&schema.AgenticMessage{
		Role: schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{
			schema.NewContentBlock(&schema.Reasoning{Text: "summary"}),
			schema.NewContentBlock(&schema.AssistantGenText{Text: "answer"}),
		},
	})
	if err != nil {
		t.Fatalf("agenticMessageToMessage: %v", err)
	}
	if msg == nil {
		t.Fatal("expected converted message")
	}
	if msg.ReasoningContent != "summary" {
		t.Fatalf("expected reasoning summary to populate ReasoningContent, got %q", msg.ReasoningContent)
	}

	events := assistantContentEvents(msg)
	if len(events) != 2 {
		t.Fatalf("expected reasoning and text events, got %#v", events)
	}
	if events[0].Kind != RuntimeEventReasoning || events[0].Text != "summary" {
		t.Fatalf("expected reasoning event first, got %#v", events[0])
	}
	if events[1].Kind != RuntimeEventText || events[1].Text != "answer" {
		t.Fatalf("expected answer event second, got %#v", events[1])
	}
}
