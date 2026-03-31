package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type styledSpan struct {
	Text  string
	Style lipgloss.Style
}

func renderMarkdown(text string, width int, baseStyle lipgloss.Style) string {
	width = maxInt(1, width)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	var rendered []string
	inCodeBlock := false
	var paragraph []string
	pendingGap := false

	ensureGapBefore := func() {
		if len(rendered) == 0 {
			pendingGap = false
			return
		}
		if strings.TrimSpace(stripANSICodes(rendered[len(rendered)-1])) == "" {
			pendingGap = false
			return
		}
		rendered = append(rendered, "")
		pendingGap = false
	}

	appendBlock := func(block []string) {
		if len(block) == 0 {
			return
		}
		if pendingGap && len(rendered) > 0 && strings.TrimSpace(stripANSICodes(rendered[len(rendered)-1])) != "" {
			rendered = append(rendered, "")
		}
		rendered = append(rendered, block...)
		pendingGap = false
	}

	flushParagraph := func() {
		if len(paragraph) == 0 {
			return
		}
		appendBlock(renderStyledParagraph(strings.Join(paragraph, " "), "", width, baseStyle))
		paragraph = nil
	}

	for idx := 0; idx < len(lines); idx++ {
		rawLine := lines[idx]
		line := strings.TrimRight(rawLine, " \t")
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```") {
			flushParagraph()
			if !inCodeBlock {
				ensureGapBefore()
			} else {
				pendingGap = len(rendered) > 0
			}
			inCodeBlock = !inCodeBlock
			continue
		}
		if inCodeBlock {
			appendBlock(renderCodeBlockLine(line, width))
			continue
		}
		if trimmed == "" {
			flushParagraph()
			pendingGap = len(rendered) > 0
			continue
		}
		if isMarkdownTableStart(lines, idx) {
			flushParagraph()
			ensureGapBefore()
			tableLines, next := collectMarkdownTable(lines, idx)
			appendBlock(renderMarkdownTable(tableLines, width, baseStyle))
			pendingGap = len(rendered) > 0
			idx = next - 1
			continue
		}
		if quote, ok := trimMarkdownPrefix(trimmed, "> "); ok {
			flushParagraph()
			ensureGapBefore()
			appendBlock(renderStyledParagraph(quote, quotePrefix(), width, quoteTextStyle))
			pendingGap = len(rendered) > 0
			continue
		}
		if heading, level, ok := trimHeading(trimmed); ok {
			flushParagraph()
			ensureGapBefore()
			appendBlock(renderStyledParagraph(heading, "", width, headingStyle(level)))
			pendingGap = len(rendered) > 0
			continue
		}
		if body, prefix, ok := trimUnorderedList(trimmed); ok {
			flushParagraph()
			appendBlock(renderStyledParagraph(body, prefix, width, baseStyle))
			continue
		}
		if body, prefix, ok := trimOrderedList(trimmed); ok {
			flushParagraph()
			appendBlock(renderStyledParagraph(body, prefix, width, baseStyle))
			continue
		}
		paragraph = append(paragraph, trimmed)
	}
	flushParagraph()
	if len(rendered) == 0 {
		return ""
	}
	return strings.Join(rendered, "\n")
}

func renderCodeBlockLine(line string, width int) []string {
	prefix := "  "
	bodyWidth := maxInt(1, width-lipgloss.Width(prefix))
	lines := wrapContent(line, bodyWidth)
	if len(lines) == 0 {
		return []string{codeBlockStyle.Render(prefix)}
	}
	output := make([]string, 0, len(lines))
	for _, wrapped := range lines {
		output = append(output, codeBlockStyle.Render(prefix+wrapped))
	}
	return output
}

func renderStyledParagraph(text, prefix string, width int, baseStyle lipgloss.Style) []string {
	width = maxInt(1, width)
	prefixWidth := lipgloss.Width(prefix)
	bodyWidth := maxInt(1, width-prefixWidth)
	spans := parseInlineMarkdown(text, baseStyle)
	lines := wrapStyledSpans(spans, bodyWidth)
	if len(lines) == 0 {
		if prefix == "" {
			return []string{""}
		}
		return []string{prefix}
	}
	output := make([]string, 0, len(lines))
	for idx, line := range lines {
		linePrefix := prefix
		if idx > 0 && prefixWidth > 0 {
			linePrefix = strings.Repeat(" ", prefixWidth)
		}
		output = append(output, linePrefix+line)
	}
	return output
}

func parseInlineMarkdown(text string, baseStyle lipgloss.Style) []styledSpan {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	var spans []styledSpan
	for len(text) > 0 {
		switch {
		case strings.HasPrefix(text, "`"):
			end := strings.Index(text[1:], "`")
			if end < 0 {
				spans = appendSpan(spans, styledSpan{Text: text, Style: baseStyle})
				return spans
			}
			code := text[1 : end+1]
			spans = appendSpan(spans, styledSpan{Text: code, Style: inlineCodeStyle})
			text = text[end+2:]
		case strings.HasPrefix(text, "**"):
			end := strings.Index(text[2:], "**")
			if end < 0 {
				spans = appendSpan(spans, styledSpan{Text: text, Style: baseStyle})
				return spans
			}
			strong := text[2 : end+2]
			spans = appendSpan(spans, styledSpan{Text: strong, Style: strongTextStyle})
			text = text[end+4:]
		default:
			nextCode := strings.Index(text, "`")
			nextStrong := strings.Index(text, "**")
			next := nextSpecialIndex(nextCode, nextStrong)
			if next < 0 {
				spans = appendSpan(spans, styledSpan{Text: text, Style: baseStyle})
				return spans
			}
			spans = appendSpan(spans, styledSpan{Text: text[:next], Style: baseStyle})
			text = text[next:]
		}
	}
	return spans
}

func wrapStyledSpans(spans []styledSpan, width int) []string {
	if len(spans) == 0 {
		return nil
	}
	width = maxInt(1, width)
	type token struct {
		Text  string
		Style lipgloss.Style
	}
	var tokens []token
	for _, span := range spans {
		fields := strings.Fields(span.Text)
		if len(fields) == 0 {
			continue
		}
		for _, field := range fields {
			tokens = append(tokens, token{Text: field, Style: span.Style})
		}
	}
	if len(tokens) == 0 {
		return nil
	}

	var lines []string
	var line strings.Builder
	lineWidth := 0
	appendLine := func() {
		lines = append(lines, line.String())
		line.Reset()
		lineWidth = 0
	}

	for _, tok := range tokens {
		tokenWidth := lipgloss.Width(tok.Text)
		if tokenWidth > width {
			parts := breakLongToken(tok.Text, width)
			for _, part := range parts {
				partWidth := lipgloss.Width(part)
				if lineWidth > 0 && lineWidth+1+partWidth > width {
					appendLine()
				}
				if lineWidth > 0 {
					line.WriteString(" ")
					lineWidth++
				}
				line.WriteString(tok.Style.Render(part))
				lineWidth += partWidth
				if lineWidth >= width {
					appendLine()
				}
			}
			continue
		}
		if lineWidth > 0 && lineWidth+1+tokenWidth > width {
			appendLine()
		}
		if lineWidth > 0 {
			line.WriteString(" ")
			lineWidth++
		}
		line.WriteString(tok.Style.Render(tok.Text))
		lineWidth += tokenWidth
	}
	if line.Len() > 0 || len(lines) == 0 {
		appendLine()
	}
	return lines
}

func breakLongToken(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	runes := []rune(text)
	var parts []string
	var current []rune
	currentWidth := 0
	for _, r := range runes {
		rw := lipgloss.Width(string(r))
		if currentWidth > 0 && currentWidth+rw > width {
			parts = append(parts, string(current))
			current = nil
			currentWidth = 0
		}
		current = append(current, r)
		currentWidth += rw
	}
	if len(current) > 0 {
		parts = append(parts, string(current))
	}
	return parts
}

func appendSpan(spans []styledSpan, span styledSpan) []styledSpan {
	if strings.TrimSpace(span.Text) == "" {
		return spans
	}
	return append(spans, span)
}

func nextSpecialIndex(indexes ...int) int {
	next := -1
	for _, idx := range indexes {
		if idx < 0 {
			continue
		}
		if next < 0 || idx < next {
			next = idx
		}
	}
	return next
}

func trimMarkdownPrefix(line, prefix string) (string, bool) {
	if strings.HasPrefix(line, prefix) {
		return strings.TrimSpace(strings.TrimPrefix(line, prefix)), true
	}
	if strings.TrimSpace(prefix) == ">" && strings.HasPrefix(line, ">") {
		return strings.TrimSpace(strings.TrimPrefix(line, ">")), true
	}
	return "", false
}

func trimHeading(line string) (string, int, bool) {
	level := 0
	for level < len(line) && level < 6 && line[level] == '#' {
		level++
	}
	if level == 0 || level >= len(line) || line[level] != ' ' {
		return "", 0, false
	}
	return strings.TrimSpace(line[level:]), level, true
}

func trimUnorderedList(line string) (string, string, bool) {
	if len(line) < 2 {
		return "", "", false
	}
	switch {
	case strings.HasPrefix(line, "- "):
		return strings.TrimSpace(line[2:]), listBulletStyle.Render("• "), true
	case strings.HasPrefix(line, "* "):
		return strings.TrimSpace(line[2:]), listBulletStyle.Render("• "), true
	case strings.HasPrefix(line, "+ "):
		return strings.TrimSpace(line[2:]), listBulletStyle.Render("• "), true
	default:
		return "", "", false
	}
}

func trimOrderedList(line string) (string, string, bool) {
	dot := strings.Index(line, ". ")
	if dot <= 0 {
		return "", "", false
	}
	number := line[:dot]
	for _, r := range number {
		if r < '0' || r > '9' {
			return "", "", false
		}
	}
	return strings.TrimSpace(line[dot+2:]), listNumberStyle.Render(number + ". "), true
}

func quotePrefix() string {
	return quotePrefixStyle.Render("▎ ")
}

func headingStyle(level int) lipgloss.Style {
	switch level {
	case 1:
		return markdownHeading1Style
	case 2:
		return markdownHeading2Style
	default:
		return markdownHeading3Style
	}
}

func isMarkdownTableStart(lines []string, index int) bool {
	if index < 0 || index+1 >= len(lines) {
		return false
	}
	return isMarkdownTableRow(strings.TrimSpace(lines[index])) && isMarkdownTableSeparator(strings.TrimSpace(lines[index+1]))
}

func collectMarkdownTable(lines []string, start int) ([]string, int) {
	end := start
	for end < len(lines) {
		trimmed := strings.TrimSpace(lines[end])
		if trimmed == "" {
			break
		}
		if end == start+1 {
			if !isMarkdownTableSeparator(trimmed) {
				break
			}
			end++
			continue
		}
		if !isMarkdownTableRow(trimmed) {
			break
		}
		end++
	}
	return lines[start:end], end
}

func isMarkdownTableRow(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	if !strings.Contains(line, "|") {
		return false
	}
	cells := parseMarkdownTableRow(line)
	return len(cells) >= 2
}

func isMarkdownTableSeparator(line string) bool {
	cells := parseMarkdownTableRow(line)
	if len(cells) < 2 {
		return false
	}
	for _, cell := range cells {
		if cell == "" {
			return false
		}
		for _, r := range cell {
			if r != '-' && r != ':' {
				return false
			}
		}
	}
	return true
}

func parseMarkdownTableRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	parts := strings.Split(line, "|")
	cells := make([]string, 0, len(parts))
	for _, part := range parts {
		cells = append(cells, strings.TrimSpace(part))
	}
	return cells
}

func renderMarkdownTable(tableLines []string, width int, baseStyle lipgloss.Style) []string {
	if len(tableLines) < 2 {
		return renderStyledParagraph(strings.Join(tableLines, " "), "", width, baseStyle)
	}
	var rows [][]string
	maxCols := 0
	for idx, line := range tableLines {
		if idx == 1 && isMarkdownTableSeparator(strings.TrimSpace(line)) {
			continue
		}
		row := parseMarkdownTableRow(line)
		if len(row) == 0 {
			continue
		}
		if len(row) > maxCols {
			maxCols = len(row)
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 || maxCols == 0 {
		return nil
	}
	for idx := range rows {
		for len(rows[idx]) < maxCols {
			rows[idx] = append(rows[idx], "")
		}
	}

	widths := make([]int, maxCols)
	for col := 0; col < maxCols; col++ {
		widths[col] = 3
		for _, row := range rows {
			cellWidth := lipgloss.Width(row[col])
			if cellWidth > widths[col] {
				widths[col] = cellWidth
			}
		}
	}
	fitTableWidths(widths, width)

	lines := []string{
		renderTableBorder("┌", "┬", "┐", widths),
	}
	for idx, row := range rows {
		lines = append(lines, renderMarkdownTableRow(row, widths, idx == 0)...)
		if idx == 0 && len(rows) > 1 {
			lines = append(lines, renderTableBorder("├", "┼", "┤", widths))
		}
	}
	lines = append(lines, renderTableBorder("└", "┴", "┘", widths))
	return lines
}

func fitTableWidths(widths []int, totalWidth int) {
	if len(widths) == 0 {
		return
	}
	minWidth := 3
	available := totalWidth - (len(widths) + 1) - (2 * len(widths))
	if available < len(widths)*minWidth {
		for idx := range widths {
			widths[idx] = minWidth
		}
		return
	}
	for sumInts(widths) > available {
		largest := 0
		for idx := 1; idx < len(widths); idx++ {
			if widths[idx] > widths[largest] {
				largest = idx
			}
		}
		if widths[largest] <= minWidth {
			break
		}
		widths[largest]--
	}
}

func sumInts(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

func renderTableBorder(left, mid, right string, widths []int) string {
	parts := make([]string, 0, len(widths))
	for _, width := range widths {
		parts = append(parts, strings.Repeat("─", width+2))
	}
	return conversationDividerStyle.Render(left + strings.Join(parts, mid) + right)
}

func renderMarkdownTableRow(row []string, widths []int, header bool) []string {
	cellLines := make([][]string, len(widths))
	maxHeight := 1
	for idx := range widths {
		text := ""
		if idx < len(row) {
			text = row[idx]
		}
		wrapped := wrapContent(text, widths[idx])
		if len(wrapped) == 0 {
			wrapped = []string{""}
		}
		cellLines[idx] = wrapped
		if len(wrapped) > maxHeight {
			maxHeight = len(wrapped)
		}
	}

	output := make([]string, 0, maxHeight)
	for lineIdx := 0; lineIdx < maxHeight; lineIdx++ {
		parts := make([]string, 0, len(widths))
		for colIdx, cell := range cellLines {
			text := ""
			if lineIdx < len(cell) {
				text = cell[lineIdx]
			}
			text = padToWidth(text, widths[colIdx])
			if header {
				text = markdownHeading3Style.Render(text)
			}
			parts = append(parts, " "+text+" ")
		}
		output = append(output, "│"+strings.Join(parts, "│")+"│")
	}
	return output
}
