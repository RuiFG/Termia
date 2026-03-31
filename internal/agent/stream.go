package agent

import (
	"bufio"
	"context"
	"io"
	"strings"
	"time"
)

type StreamReader struct {
	lines  chan string
	done   chan struct{}
	closed bool
}

func NewStreamReader(reader io.Reader) *StreamReader {
	sr := &StreamReader{
		lines: make(chan string, 256),
		done:  make(chan struct{}),
	}
	go sr.read(reader)
	return sr
}

func (r *StreamReader) read(reader io.Reader) {
	defer close(r.done)
	defer close(r.lines)

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		r.lines <- scanner.Text()
	}
}

func (r *StreamReader) NextChunk(ctx context.Context, maxLines int, wait time.Duration) (StreamChunk, error) {
	if r == nil {
		return StreamChunk{}, io.EOF
	}
	if maxLines <= 0 {
		maxLines = 120
	}
	if wait <= 0 {
		wait = 3 * time.Second
	}

	lines := make([]string, 0, maxLines)
	timer := time.NewTimer(wait)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return StreamChunk{}, ctx.Err()
		case line, ok := <-r.lines:
			if !ok {
				if len(lines) == 0 {
					r.closed = true
					return StreamChunk{}, io.EOF
				}
				r.closed = true
				return StreamChunk{
					Text:       strings.Join(lines, "\n"),
					LinesRead:  len(lines),
					EOF:        true,
					ReceivedAt: time.Now(),
				}, nil
			}
			lines = append(lines, line)
			if len(lines) >= maxLines {
				return StreamChunk{
					Text:       strings.Join(lines, "\n"),
					LinesRead:  len(lines),
					ReceivedAt: time.Now(),
				}, nil
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(wait)
		case <-timer.C:
			if len(lines) == 0 {
				return StreamChunk{TimedOut: true, ReceivedAt: time.Now()}, nil
			}
			return StreamChunk{
				Text:       strings.Join(lines, "\n"),
				LinesRead:  len(lines),
				TimedOut:   true,
				ReceivedAt: time.Now(),
			}, nil
		}
	}
}

func (r *StreamReader) CloseMessage() bool {
	return r != nil && r.closed
}
