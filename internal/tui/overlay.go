package tui

import (
	"strings"
	"unicode/utf8"

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

	return overlayContentAt(baseLines, overLines, width, height, startY)
}

func overlayContentCentered(base, over string, width, height int) string {
	baseLines := strings.Split(base, "\n")
	overLines := strings.Split(over, "\n")
	overW := maxVisibleWidth(overLines)
	overLines = padLinesToWidth(overLines, overW)
	overH := len(overLines)
	startY := (height - overH) / 2
	if startY < 0 {
		startY = 0
	}
	startX := (width - overW) / 2
	if startX < 0 {
		startX = 0
	}
	return overlayContentAtPosition(baseLines, overLines, width, height, startX, startY)
}

func overlayContentAt(baseLines, overLines []string, width, height, startY int) string {
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

	if startY < 0 {
		startY = 0
	}

	for oy := 0; oy < len(overLines); oy++ {
		by := startY + oy
		if by >= len(baseLines) {
			break
		}
		baseLines[by] = padToWidth(overLines[oy], width)
	}

	return strings.Join(baseLines, "\n")
}

func overlayContentAtPosition(baseLines, overLines []string, width, height, startX, startY int) string {
	for len(baseLines) < height {
		baseLines = append(baseLines, "")
	}
	if len(baseLines) > height {
		baseLines = baseLines[:height]
	}
	for i := range baseLines {
		baseLines[i] = padToWidth(baseLines[i], width)
	}
	if startY < 0 {
		startY = 0
	}
	if startX < 0 {
		startX = 0
	}

	for oy := 0; oy < len(overLines); oy++ {
		by := startY + oy
		if by >= len(baseLines) {
			break
		}
		overLine := overLines[oy]
		overlayWidth := lipgloss.Width(overLine)
		if overlayWidth == 0 || startX >= width {
			continue
		}
		if overlayWidth > width-startX {
			overlayWidth = width - startX
			overLine = truncate.String(overLine, uint(overlayWidth))
		}
		baseLine := padToWidth(baseLines[by], width)
		prefix := sliceStyledVisible(baseLine, 0, startX)
		suffix := sliceStyledVisible(baseLine, startX+overlayWidth, width)
		baseLines[by] = prefix + overLine + suffix
	}

	return strings.Join(baseLines, "\n")
}

func maxVisibleWidth(lines []string) int {
	maxWidth := 0
	for _, line := range lines {
		if w := lipgloss.Width(line); w > maxWidth {
			maxWidth = w
		}
	}
	return maxWidth
}

func padLinesToWidth(lines []string, width int) []string {
	if width <= 0 || len(lines) == 0 {
		return lines
	}
	padded := make([]string, len(lines))
	for i, line := range lines {
		padded[i] = padToWidth(line, width)
	}
	return padded
}

func sliceStyledVisible(s string, start, end int) string {
	if end <= start {
		return ""
	}
	start = maxInt(0, start)
	end = maxInt(start, end)
	var out strings.Builder
	curWidth := 0
	activeSGR := ""
	started := false

	for i := 0; i < len(s); {
		if seq, size, ok := nextANSIEscape(s[i:]); ok {
			activeSGR = updateActiveSGR(activeSGR, seq)
			if started {
				out.WriteString(seq)
			}
			i += size
			continue
		}

		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 0 {
			break
		}
		cell := s[i : i+size]
		rw := lipgloss.Width(cell)
		nextWidth := curWidth + rw
		if nextWidth <= start {
			curWidth = nextWidth
			i += size
			continue
		}
		if curWidth >= end {
			break
		}
		if !started {
			started = true
			if activeSGR != "" {
				out.WriteString(activeSGR)
			}
		}
		out.WriteString(cell)
		curWidth = nextWidth
		i += size
	}

	return out.String()
}

func nextANSIEscape(s string) (string, int, bool) {
	if len(s) < 2 || s[0] != '\x1b' {
		return "", 0, false
	}
	switch s[1] {
	case '[':
		for i := 2; i < len(s); i++ {
			if s[i] >= 0x40 && s[i] <= 0x7e {
				return s[:i+1], i + 1, true
			}
		}
		return s, len(s), true
	case ']':
		for i := 2; i < len(s); i++ {
			if s[i] == '\a' {
				return s[:i+1], i + 1, true
			}
			if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '\\' {
				return s[:i+2], i + 2, true
			}
		}
		return s, len(s), true
	default:
		return s[:2], 2, true
	}
}

func updateActiveSGR(active, seq string) string {
	if !strings.HasPrefix(seq, "\x1b[") || !strings.HasSuffix(seq, "m") {
		return active
	}
	params := strings.TrimSuffix(strings.TrimPrefix(seq, "\x1b["), "m")
	if params == "" || params == "0" || strings.HasPrefix(params, "0;") {
		if params == "" || params == "0" {
			return ""
		}
		return seq
	}
	return active + seq
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
