package mcptest

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"cc-switch-cli/internal/mcp"
)

// TestHelperProcess is re-exec'd by stdio probe tests. It speaks a tiny
// newline-delimited MCP dialect controlled by MCPROBE_MODE.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("MCPROBE_HELPER") != "1" {
		return
	}
	mode := os.Getenv("MCPROBE_MODE")
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	id := 0
	write := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Printf("%s\n", b)
	}
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var msg map[string]any
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		method, _ := msg["method"].(string)
		rawID := msg["id"]
		switch mode {
		case "happy":
			switch method {
			case "initialize":
				write(map[string]any{
					"jsonrpc": "2.0", "id": rawID,
					"result": map[string]any{
						"protocolVersion": "2025-06-18",
						"capabilities":    map[string]any{"tools": map[string]any{}},
						"serverInfo":      map[string]any{"name": "helper", "version": "1"},
					},
				})
			case "notifications/initialized":
				// no response
			case "tools/list":
				params, _ := msg["params"].(map[string]any)
				cursor, _ := params["cursor"].(string)
				if cursor == "" {
					write(map[string]any{
						"jsonrpc": "2.0", "id": rawID,
						"result": map[string]any{
							"tools": []any{
								map[string]any{"name": "alpha", "description": "first"},
							},
							"nextCursor": "page-2",
						},
					})
				} else {
					write(map[string]any{
						"jsonrpc": "2.0", "id": rawID,
						"result": map[string]any{
							"tools": []any{
								map[string]any{"name": "beta", "description": "second"},
							},
						},
					})
				}
			}
		case "noise":
			// Emit a stray non-JSON line then a normal initialize.
			if method == "initialize" {
				fmt.Println("this is not json")
				write(map[string]any{
					"jsonrpc": "2.0", "id": rawID,
					"result": map[string]any{
						"protocolVersion": "2025-11-25",
						"capabilities":    map[string]any{},
						"serverInfo":      map[string]any{"name": "noisy"},
					},
				})
			}
		case "bad-version":
			if method == "initialize" {
				write(map[string]any{
					"jsonrpc": "2.0", "id": rawID,
					"result": map[string]any{
						"protocolVersion": "1999-01-01",
						"capabilities":    map[string]any{},
						"serverInfo":      map[string]any{"name": "old"},
					},
				})
			}
		case "modern-reject":
			if method == "initialize" {
				write(map[string]any{
					"jsonrpc": "2.0", "id": rawID,
					"error": map[string]any{
						"code":    -32022,
						"message": "UnsupportedProtocolVersion",
						"data":    map[string]any{"supported": []string{"2026-07-28"}},
					},
				})
			}
		case "init-error":
			if method == "initialize" {
				write(map[string]any{
					"jsonrpc": "2.0", "id": rawID,
					"error": map[string]any{"code": -32603, "message": "boom"},
				})
			}
		default:
			_ = id
		}
	}
	os.Exit(0)
}

func helperEntry(mode string) mcp.Entry {
	return mcp.Entry{
		Name:  "helper-" + mode,
		Scope: mcp.ScopeUser,
		Server: &mcp.Server{
			Command: os.Args[0],
			Args:    []string{"-test.run=TestHelperProcess", "--"},
			Env: map[string]string{
				"MCPROBE_HELPER": "1",
				"MCPROBE_MODE":   mode,
			},
		},
	}
}

func TestProbeStdioHappy(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r := Probe(ctx, helperEntry("happy"), Options{WithTools: true, Timeout: 10 * time.Second})
	if r.Status != StatusOK {
		t.Fatalf("status=%s err=%s", r.Status, r.Error)
	}
	if r.ToolCount != 2 {
		t.Fatalf("tool count=%d tools=%v", r.ToolCount, r.Tools)
	}
	if r.Tools[0].Name != "alpha" || r.Tools[1].Name != "beta" {
		t.Fatalf("tools=%v", r.Tools)
	}
	if r.Transport != "stdio" {
		t.Fatalf("transport=%s", r.Transport)
	}
}

func TestProbeStdioNoise(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r := Probe(ctx, helperEntry("noise"), Options{Timeout: 10 * time.Second})
	if r.Status != StatusOK {
		t.Fatalf("status=%s err=%s", r.Status, r.Error)
	}
}

func TestProbeStdioBadVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r := Probe(ctx, helperEntry("bad-version"), Options{Timeout: 10 * time.Second})
	if r.Status != StatusFail || !strings.Contains(r.Error, "unsupported protocol version") {
		t.Fatalf("status=%s err=%s", r.Status, r.Error)
	}
}

func TestProbeStdioModernReject(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r := Probe(ctx, helperEntry("modern-reject"), Options{Timeout: 10 * time.Second})
	if r.Status != StatusFail || !strings.Contains(r.Error, "2026-07-28") {
		t.Fatalf("status=%s err=%s", r.Status, r.Error)
	}
}

func TestProbeStdioInitError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r := Probe(ctx, helperEntry("init-error"), Options{Timeout: 10 * time.Second})
	if r.Status != StatusFail || !strings.Contains(r.Error, "boom") {
		t.Fatalf("status=%s err=%s", r.Status, r.Error)
	}
}

func TestProbeWSUnsupported(t *testing.T) {
	r := Probe(context.Background(), mcp.Entry{
		Name: "ws", Scope: mcp.ScopeUser,
		Server: &mcp.Server{Type: "ws", URL: "wss://example.com/socket"},
	}, Options{})
	if r.Status != StatusUnsupported {
		t.Fatalf("status=%s", r.Status)
	}
}

func TestProbeInvalidConfig(t *testing.T) {
	r := Probe(context.Background(), mcp.Entry{
		Name: "bad", Scope: mcp.ScopeUser,
		Server: &mcp.Server{URL: "https://x/mcp"},
	}, Options{})
	if r.Status != StatusFail || !strings.Contains(r.Error, "type") {
		t.Fatalf("status=%s err=%s", r.Status, r.Error)
	}
}

func TestProbeAllOrder(t *testing.T) {
	entries := []mcp.Entry{
		helperEntry("happy"),
		{
			Name: "ws", Scope: mcp.ScopeUser,
			Server: &mcp.Server{Type: "ws", URL: "wss://example.com"},
		},
	}
	// Rename first so CacheKey uniqueness isn't an issue.
	entries[0].Name = "a-happy"
	results := ProbeAll(context.Background(), entries, Options{WithTools: false, Timeout: 10 * time.Second, Concurrency: 2}, nil)
	if len(results) != 2 {
		t.Fatalf("len=%d", len(results))
	}
	if results[0].Name != "a-happy" || results[1].Name != "ws" {
		t.Fatalf("order: %s, %s", results[0].Name, results[1].Name)
	}
	if results[0].Status != StatusOK || results[1].Status != StatusUnsupported {
		t.Fatalf("statuses: %s, %s", results[0].Status, results[1].Status)
	}
}
