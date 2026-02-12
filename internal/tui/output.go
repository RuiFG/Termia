package tui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/termia/termia/internal/db"
)

func loadOutputCmd(database *db.DB, commandID string) tea.Cmd {
	return func() tea.Msg {
		content, err := loadCommandOutput(database, commandID)
		if err != nil {
			return commandsErrorMsg{err: err}
		}
		if content == "" {
			content = "(no output)"
		}
		return outputLoadedMsg{commandID: commandID, content: content}
	}
}

func loadCommandOutput(database *db.DB, commandID string) (string, error) {
	cmd, err := database.GetCommand(commandID)
	if err != nil {
		return "", err
	}
	if cmd.StartOffset == nil || cmd.EndOffset == nil {
		return "", nil
	}
	if cmd.OutputSize != nil && *cmd.OutputSize == 0 {
		return "", nil
	}

	transcriptPath := ""
	if cmd.TranscriptPath != nil {
		transcriptPath = *cmd.TranscriptPath
	}
	if transcriptPath == "" {
		return "", fmt.Errorf("missing transcript path for command %s", cmd.ID)
	}

	data, err := readTranscriptRange(transcriptPath, *cmd.StartOffset, *cmd.EndOffset)
	if err != nil {
		return "", err
	}

	return sanitizeOutput(string(data)), nil
}

func readTranscriptRange(path string, startOffset int64, endOffset int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	if startOffset < 0 || endOffset < startOffset {
		return nil, fmt.Errorf("invalid transcript range")
	}

	if _, err := file.Seek(startOffset, io.SeekStart); err != nil {
		return nil, err
	}

	length := endOffset - startOffset
	if length == 0 {
		return []byte{}, nil
	}

	buf := make([]byte, length)
	_, err = io.ReadFull(file, buf)
	if err != nil && err != io.ErrUnexpectedEOF {
		return nil, err
	}
	return buf, nil
}

func sanitizeOutput(raw string) string {
	cleaned := stripANSICodes(raw)
	scanner := bufio.NewScanner(strings.NewReader(cleaned))
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return cleaned
	}
	return strings.Join(lines, "\n")
}

var ansiSequencePattern = regexp.MustCompile("\x1b\\[[0-?]*[ -/]*[@-~]")
var oscPattern = regexp.MustCompile("\x1b\\][^\x1b\\x07]*(?:\x1b\\\\|\x07)")
var csiSequencePattern = regexp.MustCompile("\u009b[0-?]*[ -/]*[@-~]")

func stripANSICodes(input string) string {
	cleaned := oscPattern.ReplaceAllString(input, "")
	cleaned = ansiSequencePattern.ReplaceAllString(cleaned, "")
	cleaned = csiSequencePattern.ReplaceAllString(cleaned, "")
	cleaned = strings.ReplaceAll(cleaned, "\x1b", "")
	return cleaned
}
