package mcptest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cc-switch-cli/internal/mcp"
)

func TestProbeStreamableJSON(t *testing.T) {
	var sawSession atomic.Bool
	var sawVersion atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") == "" || !strings.Contains(r.Header.Get("Accept"), "application/json") {
			t.Errorf("missing Accept dual value: %q", r.Header.Get("Accept"))
		}
		var msg jsonrpcMessage
		_ = json.NewDecoder(r.Body).Decode(&msg)
		switch msg.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "sess-1")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": 1,
				"result": map[string]any{
					"protocolVersion": "2025-06-18",
					"capabilities":    map[string]any{"tools": map[string]any{}},
					"serverInfo":      map[string]any{"name": "s"},
				},
			})
		case "notifications/initialized":
			if r.Header.Get("Mcp-Session-Id") == "sess-1" {
				sawSession.Store(true)
			}
			if r.Header.Get("MCP-Protocol-Version") == "2025-06-18" {
				sawVersion.Store(true)
			}
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": 2,
				"result": map[string]any{
					"tools": []any{map[string]any{"name": "t1", "description": "d"}},
				},
			})
		default:
			http.Error(w, "unknown", 400)
		}
	}))
	defer srv.Close()

	r := Probe(context.Background(), mcp.Entry{
		Name: "http-json", Scope: mcp.ScopeUser,
		Server: &mcp.Server{Type: "http", URL: srv.URL},
	}, Options{WithTools: true, Timeout: 5 * time.Second})
	if r.Status != StatusOK {
		t.Fatalf("status=%s err=%s", r.Status, r.Error)
	}
	if r.ToolCount != 1 || r.Tools[0].Name != "t1" {
		t.Fatalf("tools=%v count=%d", r.Tools, r.ToolCount)
	}
	if !sawSession.Load() {
		t.Fatal("session id was not echoed")
	}
	if !sawVersion.Load() {
		t.Fatal("MCP-Protocol-Version was not sent")
	}
}

func TestProbeStreamableSSEResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg jsonrpcMessage
		_ = json.NewDecoder(r.Body).Decode(&msg)
		switch msg.Method {
		case "initialize":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"protocolVersion\":\"2025-11-25\",\"capabilities\":{},\"serverInfo\":{\"name\":\"s\"}}}\n\n")
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			w.Header().Set("Content-Type", "text/event-stream")
			// A notification first, then the response.
			fmt.Fprintf(w, "data: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/message\",\"params\":{}}\n\n")
			fmt.Fprintf(w, "data: {\"jsonrpc\":\"2.0\",\"id\":2,\"result\":{\"tools\":[{\"name\":\"sse-tool\",\"description\":\"x\"}]}}\n\n")
		}
	}))
	defer srv.Close()

	r := Probe(context.Background(), mcp.Entry{
		Name: "http-sse", Scope: mcp.ScopeUser,
		Server: &mcp.Server{Type: "http", URL: srv.URL},
	}, Options{WithTools: true, Timeout: 5 * time.Second})
	if r.Status != StatusOK {
		t.Fatalf("status=%s err=%s", r.Status, r.Error)
	}
	if r.ToolCount != 1 || r.Tools[0].Name != "sse-tool" {
		t.Fatalf("tools=%v", r.Tools)
	}
}

func TestProbeHTTPAuth401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"nope"}`)
	}))
	defer srv.Close()

	r := Probe(context.Background(), mcp.Entry{
		Name: "auth", Scope: mcp.ScopeUser,
		Server: &mcp.Server{Type: "http", URL: srv.URL},
	}, Options{Timeout: 5 * time.Second})
	if r.Status != StatusFail || !strings.Contains(r.Error, "authentication") {
		t.Fatalf("status=%s err=%s", r.Status, r.Error)
	}
}

func TestProbeHTTPNoFallbackOn403(t *testing.T) {
	var gets atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			gets.Add(1)
		}
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, "no")
	}))
	defer srv.Close()

	r := Probe(context.Background(), mcp.Entry{
		Name: "forbid", Scope: mcp.ScopeUser,
		Server: &mcp.Server{Type: "http", URL: srv.URL},
	}, Options{Timeout: 5 * time.Second})
	if r.Status != StatusFail || !strings.Contains(r.Error, "forbidden") {
		t.Fatalf("status=%s err=%s", r.Status, r.Error)
	}
	if gets.Load() != 0 {
		t.Fatalf("legacy fallback should not run on 403, got %d GETs", gets.Load())
	}
}

func TestProbeLegacySSE(t *testing.T) {
	var postCount atomic.Int32
	mux := http.NewServeMux()
	// POST endpoint for messages.
	mux.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		postCount.Add(1)
		var msg jsonrpcMessage
		_ = json.NewDecoder(r.Body).Decode(&msg)
		// Legacy POST carries no response payload.
		w.WriteHeader(http.StatusAccepted)
		// Responses are delivered on the SSE stream; the test server pushes
		// them via a side channel below. For simplicity we encode the reply
		// into a package-level channel keyed by nothing — use a shared ch.
		legacyReplies <- msg
	})

	// We'll wire the SSE handler to consume legacyReplies.
	mux.HandleFunc("/sse", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "no flush", 500)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "event: endpoint\ndata: /messages\n\n")
		flusher.Flush()

		// Serve up to a few replies.
		for i := 0; i < 4; i++ {
			select {
			case req := <-legacyReplies:
				switch req.Method {
				case "initialize":
					fmt.Fprintf(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"protocolVersion\":\"2024-11-05\",\"capabilities\":{},\"serverInfo\":{\"name\":\"legacy\"}}}\n\n")
				case "notifications/initialized":
					// nothing
					continue
				case "tools/list":
					fmt.Fprintf(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":2,\"result\":{\"tools\":[{\"name\":\"legacy-tool\",\"description\":\"L\"}]}}\n\n")
				}
				flusher.Flush()
			case <-r.Context().Done():
				return
			case <-time.After(3 * time.Second):
				return
			}
		}
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Drain any leftover replies from prior tests.
	for {
		select {
		case <-legacyReplies:
		default:
			goto drained
		}
	}
drained:

	r := Probe(context.Background(), mcp.Entry{
		Name: "legacy", Scope: mcp.ScopeUser,
		Server: &mcp.Server{Type: "sse", URL: srv.URL + "/sse"},
	}, Options{WithTools: true, Timeout: 8 * time.Second})
	if r.Status != StatusOK {
		t.Fatalf("status=%s err=%s", r.Status, r.Error)
	}
	if r.ToolCount != 1 || r.Tools[0].Name != "legacy-tool" {
		t.Fatalf("tools=%v count=%d", r.Tools, r.ToolCount)
	}
	if postCount.Load() < 2 {
		t.Fatalf("expected POSTs for init+tools, got %d", postCount.Load())
	}
}

// legacyReplies carries POSTed messages from the legacy test server's
// /messages handler to its /sse handler.
var legacyReplies = make(chan jsonrpcMessage, 8)

func TestProbeHTTPFallbackToLegacy(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			// Streamable path rejects with 405 (no JSON-RPC body).
			w.WriteHeader(http.StatusMethodNotAllowed)
			fmt.Fprint(w, "use SSE")
			return
		}
		if r.Method == http.MethodGet {
			flusher := w.(http.Flusher)
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, "event: endpoint\ndata: /msg\n\n")
			flusher.Flush()
			// One initialize reply.
			select {
			case <-legacyFallbackPosts:
				fmt.Fprintf(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"protocolVersion\":\"2025-03-26\",\"capabilities\":{},\"serverInfo\":{\"name\":\"fb\"}}}\n\n")
				flusher.Flush()
			case <-r.Context().Done():
			case <-time.After(3 * time.Second):
			}
			// Drain notify + done.
			select {
			case <-legacyFallbackPosts:
			case <-time.After(1 * time.Second):
			}
		}
	})
	mux.HandleFunc("/msg", func(w http.ResponseWriter, r *http.Request) {
		legacyFallbackPosts <- struct{}{}
		w.WriteHeader(http.StatusAccepted)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	for {
		select {
		case <-legacyFallbackPosts:
		default:
			goto drained2
		}
	}
drained2:

	r := Probe(context.Background(), mcp.Entry{
		Name: "fallback", Scope: mcp.ScopeUser,
		Server: &mcp.Server{Type: "http", URL: srv.URL},
	}, Options{Timeout: 8 * time.Second})
	if r.Status != StatusOK {
		t.Fatalf("status=%s err=%s", r.Status, r.Error)
	}
}

var legacyFallbackPosts = make(chan struct{}, 8)

func TestResolveEndpoint(t *testing.T) {
	ep, err := resolveEndpoint("https://host.example/sse", "/messages?sessionId=1")
	if err != nil {
		t.Fatal(err)
	}
	if ep != "https://host.example/messages?sessionId=1" {
		t.Fatalf("got %s", ep)
	}
	if _, err := resolveEndpoint("https://host.example/sse", "https://evil.example/x"); err == nil {
		t.Fatal("cross-origin endpoint should be rejected")
	}
}
