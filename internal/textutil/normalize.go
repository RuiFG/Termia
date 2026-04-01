package textutil

import "strings"

// NormalizeLineEndings converts all CRLF and lone CR sequences to LF.
func NormalizeLineEndings(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}

// NormalizeTrimmedText normalizes line endings and trims surrounding whitespace.
func NormalizeTrimmedText(text string) string {
	return strings.TrimSpace(NormalizeLineEndings(text))
}

// NormalizeInlineText normalizes line endings and collapses all whitespace
// runs into single spaces for inline rendering contexts.
func NormalizeInlineText(text string) string {
	text = NormalizeTrimmedText(text)
	if text == "" {
		return ""
	}
	return strings.Join(strings.Fields(text), " ")
}
