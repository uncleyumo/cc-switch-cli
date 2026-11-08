package probe

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPickResponsePlainJSON(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"a"}]}}`
	resp, err := pickResponse([]byte(body), 2)
	if err != nil {
		t.Fatal(err)
	}
	var out toolsResult
	if err := decodeResult(resp, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Tools) != 1 || out.Tools[0].Name != "a" {
		t.Fatalf("tools = %+v", out.Tools)
	}
}

func TestPickResponseSSE(t *testing.T) {
	body := "event: message\n" +
		`data: {"jsonrpc":"2.0","method":"notifications/progress"}` + "\n\n" +
		"event: message\n" +
		`data: {"jsonrpc":"2.0","id":7,"result":{"ok":true}}` + "\n\n"
	resp, err := pickResponse([]byte(body), 7)
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.ID) != "7" {
		t.Fatalf("matched the wrong frame: %s", resp.ID)
	}
}

func TestPickResponseReportsMissingFrame(t *testing.T) {
	if _, err := pickResponse([]byte("data: [DONE]\n"), 3); err == nil {
		t.Fatal("expected an error when the id never appears")
	}
}

func TestTestOverHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req rpcRequest
		_ = json.Unmarshal(body, &req)
		switch req.Method {
		case "initialize":
			_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"name":"fake","version":"9"}}}`)
		case "tools/list":
			_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"one"},{"name":"two"}]}}`)
		default:
			w.WriteHeader(http.StatusAccepted)
		}
	}))
	defer srv.Close()

	res := Test(context.Background(), Target{Name: "fake", Type: "http", URL: srv.URL}, 5*time.Second)
	if !res.OK {
		t.Fatalf("probe failed: %s", res.Error)
	}
	if res.ToolCount != 2 || res.ServerName != "fake" || res.ServerVersion != "9" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestTestReportsHTTPStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	res := Test(context.Background(), Target{Name: "denied", Type: "http", URL: srv.URL}, 5*time.Second)
	if res.OK {
		t.Fatal("expected a failure")
	}
	if !strings.Contains(res.Error, "401") {
		t.Fatalf("error should name the status: %q", res.Error)
	}
}
