package tui

import (
	"strings"
	"testing"
)

func TestOverlayContentCenteredPreservesBaseOutsideOverlay(t *testing.T) {
	base := strings.Join([]string{
		"\x1b[31mLLLLLLLLLLLLLLLLLLLL\x1b[0m",
		"\x1b[31mMMMMMMMMMMMMMMMMMMMM\x1b[0m",
		"\x1b[31mNNNNNNNNNNNNNNNNNNNN\x1b[0m",
		"\x1b[31mOOOOOOOOOOOOOOOOOOOO\x1b[0m",
		"\x1b[31mPPPPPPPPPPPPPPPPPPPP\x1b[0m",
	}, "\n")
	overlay := strings.Join([]string{
		"╭──╮",
		"│ok│",
		"╰──╯",
	}, "\n")

	rendered := overlayContentCentered(base, overlay, 20, 5)
	lines := strings.Split(rendered, "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[2], "│ok│") {
		t.Fatalf("expected centered overlay content, got %q", lines[2])
	}
	if strings.TrimSpace(lines[2]) == "│ok│" {
		t.Fatalf("expected base content outside overlay to remain visible, got %q", lines[2])
	}
	plain := stripANSICodes(lines[2])
	if !strings.HasPrefix(plain, "NN") || !strings.HasSuffix(plain, "NN") {
		t.Fatalf("expected untouched base text on both sides of overlay, got %q", stripANSICodes(lines[2]))
	}
	if !strings.Contains(lines[2], "\x1b[") {
		t.Fatalf("expected ANSI color to be preserved around overlay, got %q", lines[2])
	}
}

func TestOverlayContentCenteredPadsShortOverlayLines(t *testing.T) {
	base := strings.Join([]string{
		"AAAAAAAAAAAAAAAAAAAA",
		"BBBBBBBBBBBBBBBBBBBB",
		"CCCCCCCCCCCCCCCCCCCC",
		"DDDDDDDDDDDDDDDDDDDD",
		"EEEEEEEEEEEEEEEEEEEE",
	}, "\n")
	overlay := strings.Join([]string{
		"######",
		"#x#",
		"######",
	}, "\n")

	rendered := overlayContentCentered(base, overlay, 20, 5)
	lines := strings.Split(rendered, "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(lines))
	}
	center := stripANSICodes(lines[2])
	if strings.Contains(center, "CCC#x#CCC") {
		t.Fatalf("expected short overlay line to be padded instead of leaking base content, got %q", center)
	}
	if !strings.Contains(center, "#x#") {
		t.Fatalf("expected centered overlay content, got %q", center)
	}
}
