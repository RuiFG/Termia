package tui

import (
	"strings"
	"testing"
)

func TestRenderMarkdownTableRendersGrid(t *testing.T) {
	input := strings.Join([]string{
		"| 序号 | 命令输出 |",
		"|------|----------|",
		"| 1 | hello world |",
		"| 2 | tui |",
	}, "\n")

	rendered := renderMarkdown(input, 48, assistantBodyStyle)
	if !strings.Contains(rendered, "┌") || !strings.Contains(rendered, "└") {
		t.Fatalf("expected box-drawn table, got %q", rendered)
	}
	if !strings.Contains(rendered, "序号") || !strings.Contains(rendered, "hello world") {
		t.Fatalf("expected table content preserved, got %q", rendered)
	}
}

func TestRenderMarkdownDoesNotDoubleSpacesBetweenWords(t *testing.T) {
	rendered := renderMarkdown("It seems there is a spacing issue.", 80, assistantBodyStyle)
	normalized := strings.Join(strings.Fields(rendered), " ")
	if normalized != "It seems there is a spacing issue." {
		t.Fatalf("expected single-word spacing, got %q", rendered)
	}
}

func TestRenderMarkdownStrongTextDoesNotApplyBackground(t *testing.T) {
	rendered := renderMarkdown("**bold** text", 80, assistantBodyStyle)
	if strings.Contains(rendered, "\x1b[48") {
		t.Fatalf("expected strong text without background color, got %q", rendered)
	}
}

func TestRenderMarkdownInlineCodeDoesNotApplyBackground(t *testing.T) {
	rendered := renderMarkdown("run `ls termia-linux-amd64` now", 80, assistantBodyStyle)
	if strings.Contains(rendered, "\x1b[48") {
		t.Fatalf("expected inline code without background color, got %q", rendered)
	}
}

func TestRenderMarkdownUsesLightParagraphGapForReadableLayout(t *testing.T) {
	rendered := renderMarkdown("first line\n\nsecond line", 80, assistantBodyStyle)
	if !strings.Contains(rendered, "\n\n") {
		t.Fatalf("expected a light paragraph gap, got %q", rendered)
	}
	if strings.Contains(rendered, "\n\n\n") {
		t.Fatalf("expected no oversized paragraph gap, got %q", rendered)
	}
}
