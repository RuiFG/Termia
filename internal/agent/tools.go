package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	adktool "google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type ToolRegistry struct {
	tools map[string]adktool.Tool
}

func NewToolRegistry(database CommandDB) (*ToolRegistry, error) {
	registry := &ToolRegistry{tools: make(map[string]adktool.Tool)}

	commandTool, err := NewCommandTool(database)
	if err != nil {
		return nil, err
	}
	inputTool, err := NewInputTool()
	if err != nil {
		return nil, err
	}
	commandOutputTool, err := newInspectCommandOutputTool(database)
	if err != nil {
		return nil, err
	}
	readFileTool, err := newReadFileTool()
	if err != nil {
		return nil, err
	}
	readFilesTool, err := newReadFilesTool()
	if err != nil {
		return nil, err
	}
	listDirTool, err := newListDirTool()
	if err != nil {
		return nil, err
	}
	streamReadTool, err := newStreamReadTool()
	if err != nil {
		return nil, err
	}

	for _, tool := range []adktool.Tool{
		commandTool,
		inputTool,
		commandOutputTool,
		readFileTool,
		readFilesTool,
		listDirTool,
		streamReadTool,
	} {
		registry.Register(tool)
	}

	return registry, nil
}

func (r *ToolRegistry) Register(tool adktool.Tool) {
	if tool != nil {
		r.tools[tool.Name()] = tool
	}
}

func (r *ToolRegistry) Filter(names []string) []adktool.Tool {
	if len(names) == 0 {
		return r.All()
	}
	result := make([]adktool.Tool, 0, len(names))
	for _, name := range names {
		if tool, ok := r.tools[name]; ok {
			result = append(result, tool)
		}
	}
	return result
}

func (r *ToolRegistry) All() []adktool.Tool {
	result := make([]adktool.Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		result = append(result, tool)
	}
	return result
}

type ReadFileReq struct {
	Path     string `json:"path"`
	Offset   int64  `json:"offset,omitempty"`
	MaxLines int    `json:"max_lines,omitempty"`
	MaxBytes int    `json:"max_bytes,omitempty"`
}

type ReadFileRsp struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	LinesRead int    `json:"lines_read"`
	BytesRead int    `json:"bytes_read"`
	EndOffset int64  `json:"end_offset"`
	Truncated bool   `json:"truncated"`
}

func newReadFileTool() (adktool.Tool, error) {
	return functiontool.New(
		functiontool.Config{
			Name:        "read_file",
			Description: "Read a local text file with line and byte limits.",
		},
		func(tc adktool.Context, req *ReadFileReq) (*ReadFileRsp, error) {
			return readFile(req)
		},
	)
}

func readFile(req *ReadFileReq) (*ReadFileRsp, error) {
	path, err := resolvePath(req.Path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	start := req.Offset
	if start < 0 || start > int64(len(data)) {
		start = 0
	}
	content := string(data[start:])
	maxLines := req.MaxLines
	if maxLines <= 0 {
		maxLines = 200
	}
	maxBytes := req.MaxBytes
	if maxBytes <= 0 {
		maxBytes = 32 * 1024
	}
	return clampContent(path, content, start, maxLines, maxBytes), nil
}

type ReadFilesReq struct {
	Paths    []string `json:"paths"`
	MaxLines int      `json:"max_lines,omitempty"`
	MaxBytes int      `json:"max_bytes,omitempty"`
}

type ReadFilesRsp struct {
	Files []ReadFileRsp `json:"files"`
}

func newReadFilesTool() (adktool.Tool, error) {
	return functiontool.New(
		functiontool.Config{
			Name:        "read_files",
			Description: "Read multiple local text files with limits.",
		},
		func(tc adktool.Context, req *ReadFilesReq) (*ReadFilesRsp, error) {
			files := make([]ReadFileRsp, 0, len(req.Paths))
			for _, path := range req.Paths {
				file, err := readFile(&ReadFileReq{
					Path:     path,
					MaxLines: req.MaxLines,
					MaxBytes: req.MaxBytes,
				})
				if err != nil {
					files = append(files, ReadFileRsp{
						Path:    path,
						Content: fmt.Sprintf("error: %v", err),
					})
					continue
				}
				files = append(files, *file)
			}
			return &ReadFilesRsp{Files: files}, nil
		},
	)
}

type ListDirReq struct {
	Path       string `json:"path"`
	MaxEntries int    `json:"max_entries,omitempty"`
}

type DirEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	ModTime string `json:"mod_time"`
}

type ListDirRsp struct {
	Path    string     `json:"path"`
	Entries []DirEntry `json:"entries"`
}

func newListDirTool() (adktool.Tool, error) {
	return functiontool.New(
		functiontool.Config{
			Name:        "list_dir",
			Description: "List directory entries with metadata.",
		},
		func(tc adktool.Context, req *ListDirReq) (*ListDirRsp, error) {
			path, err := resolvePath(req.Path)
			if err != nil {
				return nil, err
			}
			entries, err := os.ReadDir(path)
			if err != nil {
				return nil, err
			}
			maxEntries := req.MaxEntries
			if maxEntries <= 0 {
				maxEntries = 200
			}
			if len(entries) > maxEntries {
				entries = entries[:maxEntries]
			}
			result := make([]DirEntry, 0, len(entries))
			for _, entry := range entries {
				info, _ := entry.Info()
				dirEntry := DirEntry{
					Name:  entry.Name(),
					Path:  filepath.Join(path, entry.Name()),
					IsDir: entry.IsDir(),
				}
				if info != nil {
					dirEntry.Size = info.Size()
					dirEntry.Mode = info.Mode().String()
					dirEntry.ModTime = info.ModTime().Format(time.RFC3339)
				}
				result = append(result, dirEntry)
			}
			return &ListDirRsp{Path: path, Entries: result}, nil
		},
	)
}

type StreamReadReq struct {
	Path      string `json:"path"`
	Offset    int64  `json:"offset,omitempty"`
	Follow    bool   `json:"follow,omitempty"`
	MaxLines  int    `json:"max_lines,omitempty"`
	MaxBytes  int    `json:"max_bytes,omitempty"`
	TimeoutMs int    `json:"timeout_ms,omitempty"`
}

type StreamReadRsp struct {
	Path      string `json:"path"`
	Chunk     string `json:"chunk"`
	LinesRead int    `json:"lines_read"`
	BytesRead int    `json:"bytes_read"`
	EndOffset int64  `json:"end_offset"`
	EOF       bool   `json:"eof"`
	TimedOut  bool   `json:"timed_out"`
}

func newStreamReadTool() (adktool.Tool, error) {
	return functiontool.New(
		functiontool.Config{
			Name:        "stream_read",
			Description: "Read a log file chunk with offset, follow, line, and timeout limits.",
		},
		func(tc adktool.Context, req *StreamReadReq) (*StreamReadRsp, error) {
			path, err := resolvePath(req.Path)
			if err != nil {
				return nil, err
			}
			return streamReadFile(path, req)
		},
	)
}

func streamReadFile(path string, req *StreamReadReq) (*StreamReadRsp, error) {
	maxLines := req.MaxLines
	if maxLines <= 0 {
		maxLines = 120
	}
	maxBytes := req.MaxBytes
	if maxBytes <= 0 {
		maxBytes = 32 * 1024
	}
	timeout := time.Duration(req.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 3 * time.Second
	}

	deadline := time.Now().Add(timeout)
	for {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		start := req.Offset
		if start < 0 {
			start = 0
		}
		if start > int64(len(data)) {
			start = int64(len(data))
		}
		content := string(data[start:])
		if strings.TrimSpace(content) != "" || !req.Follow || time.Now().After(deadline) {
			chunk := clampContent(path, content, start, maxLines, maxBytes)
			return &StreamReadRsp{
				Path:      chunk.Path,
				Chunk:     chunk.Content,
				LinesRead: chunk.LinesRead,
				BytesRead: chunk.BytesRead,
				EndOffset: chunk.EndOffset,
				EOF:       !req.Follow || chunk.EndOffset >= int64(len(data)),
				TimedOut:  req.Follow && strings.TrimSpace(content) == "",
			}, nil
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func clampContent(path, content string, startOffset int64, maxLines, maxBytes int) *ReadFileRsp {
	if maxLines <= 0 {
		maxLines = 200
	}
	if maxBytes <= 0 {
		maxBytes = 32 * 1024
	}

	original := content
	lines := strings.Split(content, "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		content = strings.Join(lines, "\n")
	}
	if len(content) > maxBytes {
		content = content[:maxBytes]
	}
	return &ReadFileRsp{
		Path:      path,
		Content:   content,
		LinesRead: len(strings.Split(content, "\n")),
		BytesRead: len(content),
		EndOffset: startOffset + int64(len(content)),
		Truncated: len(content) < len(original),
	}
}

func resolvePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	if !filepath.IsAbs(path) {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		path = filepath.Join(wd, path)
	}
	path = filepath.Clean(path)
	return path, nil
}
