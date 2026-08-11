package mcptest

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"cc-switch-cli/internal/mcp"
	"cc-switch-cli/internal/version"
)

const (
	scannerMaxToken = 16 * 1024 * 1024
	stderrCap       = 4 * 1024
)

// probeStdio runs the initialize (+ optional tools/list) handshake against a
// local process over newline-delimited JSON-RPC.
func probeStdio(ctx context.Context, e mcp.Entry, opts Options, r *Result) {
	srv := e.Server
	if strings.TrimSpace(srv.Command) == "" {
		r.Status = StatusFail
		r.Error = "stdio server needs a non-empty command"
		return
	}

	cmd := exec.CommandContext(ctx, srv.Command, srv.Args...)
	cmd.Env = buildEnv(srv.Env, opts.ProjectDir)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		r.Status = StatusFail
		r.Error = fmt.Sprintf("failed to open stdin: %v", err)
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		r.Status = StatusFail
		r.Error = fmt.Sprintf("failed to open stdout: %v", err)
		return
	}
	var stderrBuf bytes.Buffer
	cmd.Stderr = &cappedWriter{w: &stderrBuf, cap: stderrCap}

	if err := cmd.Start(); err != nil {
		r.Status = StatusFail
		r.Error = fmt.Sprintf("failed to start process: %v", err)
		return
	}

	// Ensure the process is reaped and killed on exit from this function.
	var (
		waitOnce sync.Once
		waitErr  error
	)
	wait := func() error {
		waitOnce.Do(func() {
			// Close stdin first so well-behaved servers exit cleanly.
			_ = stdin.Close()
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case waitErr = <-done:
			case <-time.After(500 * time.Millisecond):
				_ = cmd.Process.Kill()
				waitErr = <-done
			}
		})
		return waitErr
	}
	defer func() { _ = wait() }()

	reader := newLineReader(stdout)
	writeLine := func(payload []byte) error {
		if _, err := stdin.Write(append(payload, '\n')); err != nil {
			return fmt.Errorf("write: %w", err)
		}
		return nil
	}

	// 1. initialize
	initParams := initializeParams{
		ProtocolVersion: offeredProtocolVersion,
		Capabilities:    map[string]any{},
		ClientInfo:      clientInfo{Name: "cc-switch-cli", Version: version.Version},
	}
	initReq, err := encodeRequest(1, "initialize", initParams)
	if err != nil {
		r.Status = StatusFail
		r.Error = fmt.Sprintf("encode initialize: %v", err)
		return
	}
	if err := writeLine(initReq); err != nil {
		r.Status = StatusFail
		r.Error = withStderr(err.Error(), &stderrBuf)
		return
	}
	initMsg, err := reader.readMatching(ctx, 1)
	if err != nil {
		r.Status = StatusFail
		r.Error = withStderr(fmt.Sprintf("initialize: %v", err), &stderrBuf)
		return
	}
	if initMsg.Error != nil {
		r.Status = StatusFail
		r.Error = withStderr(classifyInitializeError(initMsg.Error), &stderrBuf)
		return
	}
	var initRes initializeResult
	if err := json.Unmarshal(initMsg.Result, &initRes); err != nil {
		r.Status = StatusFail
		r.Error = fmt.Sprintf("initialize result: %v", err)
		return
	}
	if !isAcceptedVersion(initRes.ProtocolVersion) {
		r.Status = StatusFail
		r.Error = fmt.Sprintf("unsupported protocol version %q", initRes.ProtocolVersion)
		return
	}

	// 2. notifications/initialized
	notif, err := encodeNotification("notifications/initialized", map[string]any{})
	if err != nil {
		r.Status = StatusFail
		r.Error = fmt.Sprintf("encode initialized: %v", err)
		return
	}
	if err := writeLine(notif); err != nil {
		r.Status = StatusFail
		r.Error = withStderr(err.Error(), &stderrBuf)
		return
	}

	// Initialize succeeded.
	r.Status = StatusOK

	if !opts.WithTools {
		return
	}

	// 3. tools/list (paginated)
	tools, err := listToolsVia(ctx, func(id int, method string, params any) (*jsonrpcMessage, error) {
		req, err := encodeRequest(id, method, params)
		if err != nil {
			return nil, err
		}
		if err := writeLine(req); err != nil {
			return nil, err
		}
		return reader.readMatching(ctx, id)
	})
	if err != nil {
		r.Error = withStderr(fmt.Sprintf("tools/list: %v", err), &stderrBuf)
		r.ToolCount = -1
		return
	}
	r.Tools = tools
	r.ToolCount = len(tools)
}

// listToolsVia walks tools/list pages using the provided request/response
// function. nextCursor is followed up to 100 pages.
func listToolsVia(ctx context.Context, call func(id int, method string, params any) (*jsonrpcMessage, error)) ([]ToolInfo, error) {
	var (
		tools  []ToolInfo
		cursor string
		id     = 2
	)
	for page := 0; page < 100; page++ {
		if err := ctx.Err(); err != nil {
			return tools, err
		}
		var params any
		if cursor == "" {
			params = map[string]any{}
		} else {
			params = map[string]any{"cursor": cursor}
		}
		msg, err := call(id, "tools/list", params)
		id++
		if err != nil {
			return tools, err
		}
		if msg.Error != nil {
			return tools, msg.Error
		}
		var res toolsListResult
		if err := json.Unmarshal(msg.Result, &res); err != nil {
			return tools, fmt.Errorf("decode tools/list: %w", err)
		}
		for _, t := range res.Tools {
			name := t.Name
			desc := t.Description
			if desc == "" {
				desc = t.Title
			}
			tools = append(tools, ToolInfo{Name: name, Description: desc})
		}
		if res.NextCursor == "" {
			return tools, nil
		}
		cursor = res.NextCursor
	}
	return tools, fmt.Errorf("tools/list exceeded 100 pages")
}

func buildEnv(extra map[string]string, projectDir string) []string {
	env := os.Environ()
	if projectDir != "" {
		env = append(env, "CLAUDE_PROJECT_DIR="+projectDir)
	}
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	return env
}

func withStderr(msg string, buf *bytes.Buffer) string {
	s := strings.TrimSpace(buf.String())
	if s == "" {
		return msg
	}
	if len(s) > 512 {
		s = s[:512] + "…"
	}
	return msg + "; stderr: " + s
}

// lineReader reads newline-delimited JSON-RPC messages, skipping unparseable
// lines and notifications that do not match the request id we care about.
type lineReader struct {
	sc *bufio.Scanner
}

func newLineReader(r io.Reader) *lineReader {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), scannerMaxToken)
	return &lineReader{sc: sc}
}

func (lr *lineReader) readMatching(ctx context.Context, wantID int) (*jsonrpcMessage, error) {
	type outcome struct {
		msg *jsonrpcMessage
		err error
	}
	ch := make(chan outcome, 1)
	go func() {
		for lr.sc.Scan() {
			line := bytes.TrimSpace(lr.sc.Bytes())
			if len(line) == 0 {
				continue
			}
			// Tolerate CRLF leftovers and stray non-JSON stdout.
			if line[0] != '{' {
				continue
			}
			var msg jsonrpcMessage
			if err := json.Unmarshal(line, &msg); err != nil {
				continue
			}
			// Skip notifications and responses for other ids.
			if msg.Method != "" && len(msg.ID) == 0 {
				continue
			}
			if !idMatches(msg.ID, wantID) {
				continue
			}
			ch <- outcome{msg: &msg}
			return
		}
		if err := lr.sc.Err(); err != nil {
			ch <- outcome{err: err}
			return
		}
		ch <- outcome{err: io.EOF}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case out := <-ch:
		if out.err != nil {
			if out.err == io.EOF {
				return nil, fmt.Errorf("server closed stdout before responding")
			}
			return nil, out.err
		}
		return out.msg, nil
	}
}

// cappedWriter writes up to cap bytes then discards the rest.
type cappedWriter struct {
	w   *bytes.Buffer
	cap int
	n   int
}

func (c *cappedWriter) Write(p []byte) (int, error) {
	if c.n >= c.cap {
		return len(p), nil
	}
	remain := c.cap - c.n
	if len(p) > remain {
		_, _ = c.w.Write(p[:remain])
		c.n = c.cap
		return len(p), nil
	}
	_, _ = c.w.Write(p)
	c.n += len(p)
	return len(p), nil
}
