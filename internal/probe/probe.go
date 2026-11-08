// Package probe performs a real MCP handshake so `ccsc mcp test` can say
// whether a server is actually usable. It is read-only: it starts the server,
// asks for its tool list and exits.
package probe

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Target is one server to test.
type Target struct {
	Name    string
	Type    string // stdio | http | sse
	Command string
	Args    []string
	Env     map[string]string
	URL     string
	Headers map[string]string
}

// Result is the outcome, always returned rather than raised as an error so the
// caller can print one line per server.
type Result struct {
	Name          string
	OK            bool
	Tools         []string
	ToolCount     int
	Latency       time.Duration
	ServerName    string
	ServerVersion string
	Error         string
}

const maxTools = 200

const protocolVersion = "2025-06-18"

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type initResult struct {
	ServerInfo struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"serverInfo"`
	ProtocolVersion string `json:"protocolVersion"`
}

type toolsResult struct {
	Tools []struct {
		Name string `json:"name"`
	} `json:"tools"`
}

// Test runs the handshake against one target.
func Test(ctx context.Context, t Target, timeout time.Duration) Result {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	var (
		initRes  initResult
		toolsRes toolsResult
		err      error
	)
	switch strings.ToLower(t.Type) {
	case "", "stdio":
		initRes, toolsRes, err = viaStdio(ctx, t)
	case "http", "sse":
		initRes, toolsRes, err = viaHTTP(ctx, t)
	default:
		err = fmt.Errorf("unsupported transport %q", t.Type)
	}

	res := Result{Name: t.Name, Latency: time.Since(start)}
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.OK = true
	res.ServerName = initRes.ServerInfo.Name
	res.ServerVersion = initRes.ServerInfo.Version
	res.ToolCount = len(toolsRes.Tools)
	for i, tool := range toolsRes.Tools {
		if i >= maxTools {
			break
		}
		res.Tools = append(res.Tools, tool.Name)
	}
	return res
}

func viaStdio(ctx context.Context, t Target) (initResult, toolsResult, error) {
	var zero initResult
	if t.Command == "" {
		return zero, toolsResult{}, fmt.Errorf("stdio: no command configured")
	}
	cmd := exec.CommandContext(ctx, t.Command, t.Args...)
	cmd.Env = append(os.Environ(), envList(t.Env)...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return zero, toolsResult{}, fmt.Errorf("stdio: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return zero, toolsResult{}, fmt.Errorf("stdio: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return zero, toolsResult{}, fmt.Errorf("stdio: cannot start %s: %w", t.Command, err)
	}
	defer func() {
		_ = stdin.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	enc := json.NewEncoder(stdin)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 4<<20)
	next := func() ([]byte, error) {
		for scanner.Scan() {
			line := bytes.TrimSpace(scanner.Bytes())
			if len(line) > 0 && line[0] == '{' {
				return append([]byte(nil), line...), nil
			}
		}
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("stdio: reading stdout: %w", err)
		}
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return nil, fmt.Errorf("stdio: server closed the connection (%s)", firstLine(msg))
		}
		return nil, fmt.Errorf("stdio: server closed the connection")
	}

	send := func(req rpcRequest) error {
		req.JSONRPC = "2.0"
		if err := enc.Encode(req); err != nil {
			return fmt.Errorf("stdio: cannot write %s: %w", req.Method, err)
		}
		return nil
	}

	if err := send(rpcRequest{ID: 1, Method: "initialize", Params: initParams()}); err != nil {
		return zero, toolsResult{}, err
	}
	resp, err := await(next, 1)
	if err != nil {
		return zero, toolsResult{}, err
	}
	var initRes initResult
	if err := decodeResult(resp, &initRes); err != nil {
		return zero, toolsResult{}, fmt.Errorf("stdio: initialize: %w", err)
	}
	if err := send(rpcRequest{Method: "notifications/initialized"}); err != nil {
		return zero, toolsResult{}, err
	}
	if err := send(rpcRequest{ID: 2, Method: "tools/list", Params: map[string]any{}}); err != nil {
		return zero, toolsResult{}, err
	}
	resp, err = await(next, 2)
	if err != nil {
		return zero, toolsResult{}, err
	}
	var toolsRes toolsResult
	if err := decodeResult(resp, &toolsRes); err != nil {
		return zero, toolsResult{}, fmt.Errorf("stdio: tools/list: %w", err)
	}
	return initRes, toolsRes, nil
}

func viaHTTP(ctx context.Context, t Target) (initResult, toolsResult, error) {
	var zero initResult
	if t.URL == "" {
		return zero, toolsResult{}, fmt.Errorf("http: no url configured")
	}
	client := &http.Client{Timeout: 20 * time.Second}

	call := func(req rpcRequest) (*rpcResponse, error) {
		req.JSONRPC = "2.0"
		body, err := json.Marshal(req)
		if err != nil {
			return nil, err
		}
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.URL, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "application/json, text/event-stream")
		for k, v := range t.Headers {
			httpReq.Header.Set(k, v)
		}
		resp, err := client.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("http: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return nil, fmt.Errorf("http: server returned %s", resp.Status)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		if err != nil {
			return nil, fmt.Errorf("http: %w", err)
		}
		return pickResponse(data, req.ID)
	}

	resp, err := call(rpcRequest{ID: 1, Method: "initialize", Params: initParams()})
	if err != nil {
		return zero, toolsResult{}, err
	}
	var initRes initResult
	if err := decodeResult(resp, &initRes); err != nil {
		return zero, toolsResult{}, fmt.Errorf("http: initialize: %w", err)
	}
	if _, err := call(rpcRequest{Method: "notifications/initialized"}); err != nil {
		// Some servers answer this with 202 and no body; that is fine.
		_ = err
	}
	resp, err = call(rpcRequest{ID: 2, Method: "tools/list", Params: map[string]any{}})
	if err != nil {
		return zero, toolsResult{}, err
	}
	var toolsRes toolsResult
	if err := decodeResult(resp, &toolsRes); err != nil {
		return zero, toolsResult{}, fmt.Errorf("http: tools/list: %w", err)
	}
	return initRes, toolsRes, nil
}

func initParams() map[string]any {
	return map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "ccsc", "version": "2"},
	}
}

func await(next func() ([]byte, error), id int) (*rpcResponse, error) {
	for {
		line, err := next()
		if err != nil {
			return nil, err
		}
		var msg rpcResponse
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		if !matchesID(msg.ID, id) {
			continue
		}
		return &msg, nil
	}
}

// pickResponse handles both a plain JSON body and an SSE stream.
func pickResponse(data []byte, id int) (*rpcResponse, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var msg rpcResponse
		if err := json.Unmarshal(trimmed, &msg); err != nil {
			return nil, fmt.Errorf("http: response is not valid JSON: %w", err)
		}
		return &msg, nil
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var msg rpcResponse
		if err := json.Unmarshal([]byte(payload), &msg); err != nil {
			continue
		}
		if id == 0 || matchesID(msg.ID, id) {
			return &msg, nil
		}
	}
	return nil, fmt.Errorf("http: no JSON-RPC response in the reply")
}

func matchesID(raw json.RawMessage, id int) bool {
	if len(raw) == 0 {
		return false
	}
	var got int
	if err := json.Unmarshal(raw, &got); err != nil {
		return false
	}
	return got == id
}

func decodeResult(resp *rpcResponse, dst any) error {
	if resp.Error != nil {
		return fmt.Errorf("%s (code %d)", resp.Error.Message, resp.Error.Code)
	}
	if len(resp.Result) == 0 {
		return fmt.Errorf("response has no result")
	}
	return json.Unmarshal(resp.Result, dst)
}

func envList(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 160 {
		s = s[:160] + "..."
	}
	return s
}
