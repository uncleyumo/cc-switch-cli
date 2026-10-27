package tester

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			body = nil
		}
		// Parallel providers hit the same server; hand the handler its own
		// snapshot and publish the latest one under a lock so -race is clean.
		snapshot := &recordedRequest{
			authHeader: r.Header.Get("authorization"),
			apiKey:     r.Header.Get("x-api-key"),
			body:       body,
		}
		mu.Lock()
		*rec = *snapshot
		mu.Unlock()
		handler(w, r, snapshot)
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

func TestClientTestsEveryConfiguredModel(t *testing.T) {
	var got []string
	srv, _ := newSSEServer(t, func(w http.ResponseWriter, r *http.Request, rec *recordedRequest) {
		var req messagesRequest
		_ = json.Unmarshal(rec.body, &req)
		got = append(got, req.Model)
		writeSSE(w, []string{
			`event: message_start
data: {"type":"message_start","message":{}}`,
			`event: message_stop
data: {"type":"message_stop"}`,
		})
	})
	m := &model.Model{Env: model.Env{
		BaseURL:            srv.URL,
		AuthToken:          "tok",
		Model:              "opus",
		DefaultOpusModel:   "opus-concrete",
		DefaultSonnetModel: "sonnet-concrete",
		DefaultHaikuModel:  "sonnet-concrete", // duplicate of sonnet: tested once
	}}
	client := NewClient(Options{Checks: CheckStream, Timeout: 5 * time.Second})
	res := client.TestProvider(context.Background(), "srv", m)

	if len(res) != 2 {
		t.Fatalf("expected one result per distinct configured model, got %+v", res)
	}
	if strings.Join(got, ",") != "opus-concrete,sonnet-concrete" {
		t.Fatalf("requested models = %v", got)
	}
	if res[0].ModelID != "opus-concrete" || res[0].ModelSlot != "default+opus" {
		t.Errorf("first result = %+v", res[0])
	}
	if res[1].ModelID != "sonnet-concrete" || res[1].ModelSlot != "sonnet+haiku" {
		t.Errorf("second result = %+v", res[1])
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

func TestClientHTTPFailClassifiesKind(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		status   int
		wantKind ErrorKind
	}{
		{"401 anthropic shape", `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key tok-shouldnotleak-1234"}}`, http.StatusUnauthorized, ErrKindAuth},
		{"429", `{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`, http.StatusTooManyRequests, ErrKindRateLimit},
		{"500 plain", "boom", http.StatusInternalServerError, ErrKindServerError},
		{"504", "gateway timeout", http.StatusGatewayTimeout, ErrKindTimeout},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv, _ := newSSEServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
				w.WriteHeader(c.status)
				_, _ = io.WriteString(w, c.body)
			})
			m := &model.Model{Env: model.Env{BaseURL: srv.URL, Model: "m", AuthToken: "tok-shouldnotleak-1234"}}
			client := NewClient(Options{Checks: CheckStream, Timeout: 5 * time.Second})
			res := client.TestProvider(context.Background(), "srv", m)
			if len(res) != 1 || res[0].Status == StatusOK {
				t.Fatalf("expected one failing result, got %+v", res)
			}
			if res[0].ErrorKind != c.wantKind {
				t.Errorf("ErrorKind = %q, want %q (err=%q)", res[0].ErrorKind, c.wantKind, res[0].Error)
			}
			if strings.Contains(res[0].Error, "tok-shouldnotleak-1234") {
				t.Errorf("raw token leaked into error: %s", res[0].Error)
			}
		})
	}
}

func TestClientTimeoutClassified(t *testing.T) {
	srv, _ := newSSEServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		time.Sleep(300 * time.Millisecond)
		writeSSE(w, []string{`event: message_stop
data: {"type":"message_stop"}`})
	})
	m := &model.Model{Env: model.Env{BaseURL: srv.URL, Model: "m", AuthToken: "tok"}}
	client := NewClient(Options{Checks: CheckStream, Timeout: 30 * time.Millisecond})
	res := client.TestProvider(context.Background(), "srv", m)
	if len(res) != 1 || res[0].Status != StatusFail {
		t.Fatalf("expected one fail result, got %+v", res)
	}
	if res[0].ErrorKind != ErrKindTimeout {
		t.Errorf("ErrorKind = %q, want timeout (err=%q)", res[0].ErrorKind, res[0].Error)
	}
}

func TestClientUnreachableClassifiedNetwork(t *testing.T) {
	srv := httptest.NewServer(http.NewServeMux())
	url := srv.URL
	srv.Close() // leave a closed port behind so the dial fails

	m := &model.Model{Env: model.Env{BaseURL: url, Model: "m", AuthToken: "tok"}}
	client := NewClient(Options{Checks: CheckStream, Timeout: 5 * time.Second})
	res := client.TestProvider(context.Background(), "srv", m)
	if len(res) != 1 || res[0].Status != StatusFail {
		t.Fatalf("expected one fail result, got %+v", res)
	}
	if res[0].ErrorKind != ErrKindNetwork {
		t.Errorf("ErrorKind = %q, want network (err=%q)", res[0].ErrorKind, res[0].Error)
	}
}

func TestClientInStreamErrorEventClassified(t *testing.T) {
	// Gateways may return HTTP 200 and deliver the failure as an SSE error
	// event; the apiType inside it must drive the classification.
	srv, _ := newSSEServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		writeSSE(w, []string{
			`event: message_start
data: {"type":"message_start","message":{}}`,
			`event: error
data: {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`,
		})
	})
	m := &model.Model{Env: model.Env{BaseURL: srv.URL, Model: "m", AuthToken: "tok"}}
	client := NewClient(Options{Checks: CheckStream, Timeout: 5 * time.Second})
	res := client.TestProvider(context.Background(), "srv", m)
	if len(res) != 1 || res[0].Status != StatusFail {
		t.Fatalf("expected one fail result, got %+v", res)
	}
	if res[0].ErrorKind != ErrKindOverloaded {
		t.Errorf("ErrorKind = %q, want overloaded (err=%q)", res[0].ErrorKind, res[0].Error)
	}
}

func TestClientEmptyStreamClassified(t *testing.T) {
	srv, _ := newSSEServer(t, func(w http.ResponseWriter, r *http.Request, _ *recordedRequest) {
		w.Header().Set("content-type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
	})
	m := &model.Model{Env: model.Env{BaseURL: srv.URL, Model: "m", AuthToken: "tok"}}
	client := NewClient(Options{Checks: CheckStream, Timeout: 5 * time.Second})
	res := client.TestProvider(context.Background(), "srv", m)
	if len(res) != 1 || res[0].Status != StatusFail {
		t.Fatalf("expected one fail result, got %+v", res)
	}
	if res[0].ErrorKind != ErrKindEmptyResp {
		t.Errorf("ErrorKind = %q, want empty_response (err=%q)", res[0].ErrorKind, res[0].Error)
	}
}
