package tester

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cc-switch-cli/internal/model"
)

// streamServer echoes a canned SSE stream. It also records the last
// Authorization / x-api-key header so tests can inspect auth handling.
type recordedRequest struct {
	authHeader string
	apiKey     string
	body       json.RawMessage
}

func newSSEServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request, rec *recordedRequest)) (*httptest.Server, *recordedRequest) {
	rec := &recordedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.authHeader = r.Header.Get("authorization")
		rec.apiKey = r.Header.Get("x-api-key")
		body, err := io.ReadAll(r.Body)
		if err == nil {
			rec.body = body
		}
		handler(w, r, rec)
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

func writeSSE(w http.ResponseWriter, events []string) {
	w.Header().Set("content-type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	f, _ := w.(http.Flusher)
	for _, e := range events {
		_, _ = io.WriteString(w, e+"\n\n")
		if f != nil {
			f.Flush()
		}
	}
}

func TestClientStreamOK(t *testing.T) {
	srv, rec := newSSEServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		writeSSE(w, []string{
			`event: message_start
data: {"type":"message_start","message":{}}`,
			`event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"pong"}}`,
			`event: message_stop
data: {"type":"message_stop"}`,
		})
	})
	m := &model.Model{Env: model.Env{BaseURL: srv.URL, Model: "test-model", AuthToken: "tok"}}
	client := NewClient(Options{Checks: CheckStream, Timeout: 5 * time.Second})
	res := client.TestProvider(context.Background(), "srv", m)
	if len(res) != 1 || res[0].Status != StatusOK {
		t.Fatalf("expected one OK result, got %+v", res)
	}
	if rec.authHeader == "" || !strings.HasPrefix(rec.authHeader, "Bearer ") {
		t.Fatalf("expected Bearer auth by default, got %q", rec.authHeader)
	}
}

func TestClientAuthFallback(t *testing.T) {
	// First call: no x-api-key header → respond 401. Second call: presence of
	// x-api-key → succeed with a stream.
	attempt := 0
	srv, _ := newSSEServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		attempt++
		if r.Header.Get("x-api-key") == "" {
			http.Error(w, `{"error":{"message":"missing api key"}}`, http.StatusUnauthorized)
			return
		}
		writeSSE(w, []string{
			`event: message_start
data: {"type":"message_start","message":{}}`,
			`event: message_stop
data: {"type":"message_stop"}`,
		})
	})
	m := &model.Model{Env: model.Env{BaseURL: srv.URL, Model: "m", AuthToken: "tok"}}
	client := NewClient(Options{Checks: CheckStream, Timeout: 5 * time.Second, Auth: AuthAuto})
	res := client.TestProvider(context.Background(), "srv", m)
	if len(res) != 1 || res[0].Status != StatusOK {
		t.Fatalf("expected OK after fallback, got %+v", res)
	}
	if attempt != 2 {
		t.Fatalf("expected two attempts (bearer→x-api-key), got %d", attempt)
	}
}

func TestClientToolUnsupported(t *testing.T) {
	srv, _ := newSSEServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		var req map[string]any
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)
		if _, ok := req["tools"]; ok {
			http.Error(w, `{"error":{"message":"tool_choice is not supported"}}`, http.StatusBadRequest)
			return
		}
		writeSSE(w, []string{
			`event: message_start
data: {"type":"message_start"}`,
			`event: message_stop
data: {"type":"message_stop"}`,
		})
	})
	m := &model.Model{Env: model.Env{BaseURL: srv.URL, Model: "m", AuthToken: "tok"}}
	client := NewClient(Options{Checks: CheckTool, Timeout: 5 * time.Second})
	res := client.TestProvider(context.Background(), "srv", m)
	if len(res) != 1 {
		t.Fatalf("expected 1 result, got %d", len(res))
	}
	if res[0].Status != StatusUnsupported {
		t.Fatalf("expected StatusUnsupported, got %q (err=%q)", res[0].Status, res[0].Error)
	}
}

func TestClientHTTPFailScrubsToken(t *testing.T) {
	srv, _ := newSSEServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		http.Error(w, `{"error":{"message":"bad request from tok-shouldnotleak-1234"}}`, http.StatusBadRequest)
	})
	m := &model.Model{Env: model.Env{BaseURL: srv.URL, Model: "m", AuthToken: "tok-shouldnotleak-1234"}}
	client := NewClient(Options{Checks: CheckStream, Timeout: 5 * time.Second})
	res := client.TestProvider(context.Background(), "srv", m)
	if len(res) != 1 || res[0].Status != StatusFail {
		t.Fatalf("expected fail, got %+v", res)
	}
	if strings.Contains(res[0].Error, "tok-shouldnotleak-1234") {
		t.Fatalf("raw token leaked into error: %s", res[0].Error)
	}
}

func TestClientTestAllRunsInParallel(t *testing.T) {
	srv, _ := newSSEServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		writeSSE(w, []string{
			`event: message_start
data: {"type":"message_start"}`,
			`event: message_stop
data: {"type":"message_stop"}`,
		})
	})
	models := map[string]*model.Model{}
	for _, n := range []string{"a", "b", "c", "d"} {
		models[n] = &model.Model{Env: model.Env{BaseURL: srv.URL, Model: "m", AuthToken: "t"}}
	}
	client := NewClient(Options{Checks: CheckStream, Timeout: 5 * time.Second, Concurrency: 4})
	got := client.TestAll(context.Background(), models, nil)
	if len(got) != len(models) {
		t.Fatalf("expected %d results, got %d", len(models), len(got))
	}
	for _, r := range got {
		if r.Status != StatusOK {
			t.Fatalf("expected all OK, got %+v", r)
		}
	}
}
