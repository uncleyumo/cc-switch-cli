package tester

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// sseEvent is the shape we care about across Anthropic SSE frames.
type sseEvent struct {
	Type         string        `json:"type"`
	Index        int           `json:"index"`
	ContentBlock *contentBlock `json:"content_block,omitempty"`
	Delta        *delta        `json:"delta,omitempty"`
	Error        *apiError     `json:"error,omitempty"`
}

type contentBlock struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
	ID   string `json:"id,omitempty"`
}

type delta struct {
	Type       string `json:"type,omitempty"`
	Text       string `json:"text,omitempty"`
	StopReason string `json:"stop_reason,omitempty"`
}

type apiError struct {
	Type    string `json:"type,omitempty"`
	Message string `json:"message,omitempty"`
}

// readSSE reads a Server-Sent Events stream and invokes onEvent for each
// parsed data payload. When onEvent returns done=true the loop exits.
//
// The reader tolerates:
//   - CRLF or LF line endings
//   - comment lines (":..." keep-alives)
//   - multi-line "data:" values (concatenated with "\n" per the SSE spec)
//   - "event:" prefixes (Anthropic mirrors these in the JSON's "type" field)
func readSSE(ctx context.Context, body io.Reader, onEvent func(ev sseEvent) (done bool, err error)) error {
	scanner := bufio.NewScanner(body)
	// Anthropic events can be a few KB (message_start metadata); grow the buffer.
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1<<20)

	var dataParts []string
	flush := func() (bool, error) {
		if len(dataParts) == 0 {
			return false, nil
		}
		payload := strings.Join(dataParts, "\n")
		dataParts = dataParts[:0]
		if payload == "" {
			return false, nil
		}
		if payload == "[DONE]" {
			return true, nil
		}
		var ev sseEvent
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			// Non-JSON data lines aren't fatal; skip.
			return false, nil
		}
		return onEvent(ev)
	}

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" {
			done, err := flush()
			if err != nil {
				return err
			}
			if done {
				return nil
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			// Anthropic includes the same info in the JSON payload; safe to ignore.
			continue
		}
		if strings.HasPrefix(line, "data:") {
			part := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			dataParts = append(dataParts, part)
		}
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("sse read failed: %w", err)
	}
	// Flush any trailing event without a blank-line terminator.
	if _, err := flush(); err != nil {
		return err
	}
	return nil
}

// containsToolUse peeks at raw JSON to decide whether a content_block_start
// frame declares a tool_use block. Kept separate for reuse if we need it.
func containsToolUse(raw []byte) bool {
	return bytes.Contains(raw, []byte(`"tool_use"`))
}
