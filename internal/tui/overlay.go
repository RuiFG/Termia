package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/truncate"
)

// overlayContent overlays the `over` string on top of `base`, positioning
// the overlay at the bottom of the base content area.
//
// Both base and over should be plain content strings (no panel borders).
// Every output line is guaranteed to be exactly `width` visible cells wide,
// which prevents ghost characters in Bubbletea's diff-based renderer.
//
// Parameters:
//   - base:   the content string to overlay on (e.g., agent or preview View())
//   - over:   the overlay string (e.g., slash menu with its own border)
//   - width:  the expected visible width of every content line
//   - height: the expected number of lines in the output
func overlayContent(base, over string, width, height int) string {
	baseLines := strings.Split(base, "\n")
	overLines := strings.Split(over, "\n")

	// Normalize base to exactly `height` lines, each exactly `width` visible cells wide.
	for len(baseLines) < height {
		baseLines = append(baseLines, "")
	}
	if len(baseLines) > height {
		baseLines = baseLines[:height]
	}
	for i := range baseLines {
		baseLines[i] = padToWidth(baseLines[i], width)
	}

	// Position overlay at the bottom of the content area
	overH := len(overLines)
	startY := height - overH
	if startY < 0 {
		startY = 0
	}

	// Replace lines in the overlay zone
	for oy := 0; oy < len(overLines); oy++ {
		by := startY + oy
		if by >= len(baseLines) {
			break
		}
		// Pad/truncate overlay line to exactly `width`
		baseLines[by] = padToWidth(overLines[oy], width)
	}

	return strings.Join(baseLines, "\n")
}

// padToWidth ensures a string is exactly `width` visible cells wide.
// Pads with spaces if shorter, truncates (ANSI-aware) if longer.
func padToWidth(s string, width int) string {
	w := lipgloss.Width(s)
	if w < width {
		return s + strings.Repeat(" ", width-w)
	}
	if w > width {
		return truncate.String(s, uint(width))
	}
	return s
}
