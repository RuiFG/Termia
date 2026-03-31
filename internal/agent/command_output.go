package agent

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	adktool "google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type InspectCommandOutputReq struct {
	CommandID  string `json:"command_id"`
	Offset     int64  `json:"offset,omitempty"`
	MaxLines   int    `json:"max_lines,omitempty"`
	MaxBytes   int    `json:"max_bytes,omitempty"`
	Query      string `json:"query,omitempty"`
	IgnoreCase bool   `json:"ignore_case,omitempty"`
	Regex      bool   `json:"regex,omitempty"`
	Before     int    `json:"before,omitempty"`
	After      int    `json:"after,omitempty"`
	MaxMatches int    `json:"max_matches,omitempty"`
}

type InspectCommandOutputRsp struct {
	CommandID           string `json:"command_id"`
	Command             string `json:"command"`
	Cwd                 string `json:"cwd,omitempty"`
	TranscriptAvailable bool   `json:"transcript_available"`
	OutputSize          *int64 `json:"output_size,omitempty"`
	Chunk               string `json:"chunk,omitempty"`
	LinesRead           int    `json:"lines_read,omitempty"`
	BytesRead           int    `json:"bytes_read,omitempty"`
	EndOffset           int64  `json:"end_offset,omitempty"`
	Truncated           bool   `json:"truncated,omitempty"`
	Query               string `json:"query,omitempty"`
	MatchCount          int    `json:"match_count,omitempty"`
	Excerpt             string `json:"excerpt,omitempty"`
}

func newInspectCommandOutputTool(database CommandDB) (adktool.Tool, error) {
	return functiontool.New(
		functiontool.Config{
			Name:        "inspect_command_output",
			Description: "Inspect a recorded command output by command_id. Supports chunked reads and grep-like search.",
		},
		func(tc adktool.Context, req *InspectCommandOutputReq) (*InspectCommandOutputRsp, error) {
			return inspectCommandOutput(database, req)
		},
	)
}

func inspectCommandOutput(database CommandDB, req *InspectCommandOutputReq) (*InspectCommandOutputRsp, error) {
	if database == nil {
		return nil, fmt.Errorf("command database is unavailable")
	}
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	commandID := strings.TrimSpace(req.CommandID)
	if commandID == "" {
		return nil, fmt.Errorf("command_id is required")
	}
	command, err := database.GetCommand(commandID)
	if err != nil {
		return nil, err
	}
	response := &InspectCommandOutputRsp{
		CommandID:           command.ID,
		Command:             command.Command,
		Cwd:                 command.Cwd,
		TranscriptAvailable: command.TranscriptPath != nil,
		OutputSize:          command.OutputSize,
	}
	if command.TranscriptPath == nil || strings.TrimSpace(*command.TranscriptPath) == "" {
		return response, nil
	}
	data, err := os.ReadFile(strings.TrimSpace(*command.TranscriptPath))
	if err != nil {
		return nil, fmt.Errorf("read command output %s: %w", commandID, err)
	}
	content := sanitizeOutput(string(data))
	if strings.TrimSpace(req.Query) != "" {
		excerpt, matches, err := searchCommandOutput(content, req)
		if err != nil {
			return nil, err
		}
		response.Query = strings.TrimSpace(req.Query)
		response.MatchCount = matches
		response.Excerpt = excerpt
		return response, nil
	}
	maxLines := req.MaxLines
	if maxLines <= 0 {
		maxLines = 200
	}
	maxBytes := req.MaxBytes
	if maxBytes <= 0 {
		maxBytes = 32 * 1024
	}
	start := req.Offset
	if start < 0 {
		start = 0
	}
	if start > int64(len(content)) {
		start = int64(len(content))
	}
	chunk := clampContent("command:"+commandID, content[start:], start, maxLines, maxBytes)
	response.Chunk = chunk.Content
	response.LinesRead = chunk.LinesRead
	response.BytesRead = chunk.BytesRead
	response.EndOffset = chunk.EndOffset
	response.Truncated = chunk.Truncated
	return response, nil
}

func searchCommandOutput(content string, req *InspectCommandOutputReq) (string, int, error) {
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return "", 0, nil
	}
	before := req.Before
	if before < 0 {
		before = 0
	}
	after := req.After
	if after < 0 {
		after = 0
	}
	maxMatches := req.MaxMatches
	if maxMatches <= 0 {
		maxMatches = 20
	}

	var compiled *regexp.Regexp
	if req.Regex {
		pattern := query
		if req.IgnoreCase {
			pattern = "(?i)" + pattern
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return "", 0, fmt.Errorf("compile query regex: %w", err)
		}
		compiled = re
	}

	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	var sb strings.Builder
	lastPrinted := -2
	matchCount := 0
	for i, line := range lines {
		if !matchesCommandOutputLine(line, query, req.IgnoreCase, compiled) {
			continue
		}
		matchCount++
		if matchCount > maxMatches {
			break
		}
		start := maxInt(0, i-before)
		end := minInt(len(lines)-1, i+after)
		if sb.Len() > 0 && start > lastPrinted+1 {
			sb.WriteString("--\n")
		}
		for j := start; j <= end; j++ {
			if j <= lastPrinted {
				continue
			}
			separator := "-"
			if j == i {
				separator = ":"
			}
			sb.WriteString(fmt.Sprintf("%d%s %s\n", j+1, separator, lines[j]))
			lastPrinted = j
		}
	}
	return strings.TrimRight(sb.String(), "\n"), minInt(matchCount, maxMatches), nil
}

func matchesCommandOutputLine(line, query string, ignoreCase bool, compiled *regexp.Regexp) bool {
	if compiled != nil {
		return compiled.MatchString(line)
	}
	if ignoreCase {
		return strings.Contains(strings.ToLower(line), strings.ToLower(query))
	}
	return strings.Contains(line, query)
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
