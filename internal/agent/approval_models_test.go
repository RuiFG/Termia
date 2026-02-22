package agent

import (
	"context"
	"strings"
	"testing"
)

type stubApprovalProvider struct {
	decision ApprovalDecision
}

func (s stubApprovalProvider) RequestApproval(_ context.Context, _ ApprovalPrompt) (ApprovalDecision, error) {
	return s.decision, nil
}

func TestAskOptionLength(t *testing.T) {
	option := AskOption{Title: strings.Repeat("a", AskOptionTitleMaxLen+1)}
	if err := ValidateAskOption(option); err == nil {
		t.Fatalf("expected error for long title")
	}
}

func TestRequestApprovalOrEnqueueUsesProvider(t *testing.T) {
	provider := stubApprovalProvider{decision: ApprovalDecision{Type: ApprovalDecisionApprove}}
	decision, err := requestApprovalOrEnqueue(nil, "echo hi", provider)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Type != ApprovalDecisionApprove {
		t.Fatalf("expected approve decision, got %s", decision.Type)
	}
}

func TestNormalizeAskQuestionFillsDefaults(t *testing.T) {
	question := AskQuestion{
		Question: "What next?",
		Options: []AskOption{
			{Title: "  Option One  ", Description: strings.Repeat("x", AskOptionDescMaxLen+5)},
		},
	}
	norm, err := NormalizeAskQuestion(question)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if norm.Header == "" {
		t.Fatalf("expected header to be filled")
	}
	if len(norm.Options) == 0 {
		t.Fatalf("expected options to be present")
	}
	hasType := false
	for _, option := range norm.Options {
		if option.Title == AskTypeYourAnswerTitle {
			hasType = true
		}
		if len([]rune(option.Title)) > AskOptionTitleMaxLen {
			t.Fatalf("expected option title to be truncated")
		}
		if len([]rune(option.Description)) > AskOptionDescMaxLen {
			t.Fatalf("expected option description to be truncated")
		}
	}
	if !hasType {
		t.Fatalf("expected Type Your Answer option")
	}
}
