package mcptest

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cc-switch-cli/internal/mcp"
	"cc-switch-cli/internal/version"
)

// probeHTTP handles Streamable HTTP and legacy HTTP+SSE. type "sse" always
// uses the legacy path; type "http" tries streamable first and falls back on
// 400/404/405/406 whose body is not a modern JSON-RPC error.
func probeHTTP(ctx context.Context, e mcp.Entry, opts Options, r *Result) {
	srv := e.Server
	if strings.TrimSpace(srv.URL) == "" {
		r.Status = StatusFail
		r.Error = "missing url"
		return
	}
	if srv.Transport() == mcp.TransportSSE {
		probeLegacySSE(ctx, e, opts, r)
		return
	}

	client := &http.Client{Timeout: 0} // rely on ctx
	sessionID, negotiated, err := streamableInitialize(ctx, client, srv)
	if err != nil {
		if shouldFallbackLegacy(err) {
			probeLegacySSE(ctx, e, opts, r)
			return
		}
		r.Status = StatusFail
		r.Error = err.Error()
		return
	}
	if !isAcceptedVersion(negotiated) {
		r.Status = StatusFail
		r.Error = fmt.Sprintf("unsupported protocol version %q", negotiated)
		return
	}

	// notifications/initialized
	if err := streamableNotify(ctx, client, srv, sessionID, negotiated); err != nil {
		// Non-fatal for a few servers, but most require it. Treat as soft fail
		// only when the status is already looking healthy? Spec requires it.
		r.Status = StatusFail
		r.Error = fmt.Sprintf("notifications/initialized: %v", err)
		return
	}

	r.Status = StatusOK
	if !opts.WithTools {
		return
	}

	tools, err := listToolsVia(ctx, func(id int, method string, params any) (*jsonrpcMessage, error) {
		return streamableCall(ctx, client, srv, sessionID, negotiated, id, method, params)
	})
	if err != nil {
		r.Error = fmt.Sprintf("tools/list: %v", err)
		r.ToolCount = -1
		return
	}
	r.Tools = tools
	r.ToolCount = len(tools)
}

// httpProbeError carries enough context for the fallback decision.
type httpProbeError struct {
	status int
	body   string
	msg    string
	auth   bool
}

func (e *httpProbeError) Error() string { return e.msg }

func shouldFallbackLegacy(err error) bool {
	he, ok := err.(*httpProbeError)
	if !ok {
		return false
	}
	if he.auth {
		return false
	}
	switch he.status {
	case 400, 404, 405, 406:
		// Do not fall back when the body is a modern JSON-RPC error (e.g.
		// UnsupportedProtocolVersion) — that is a semantic rejection, not a
		// transport mismatch.
		if looksLikeJSONRPCError(he.body) {
			return false
		}
		return true
	}
	return false
}

func looksLikeJSONRPCError(body string) bool {
	body = strings.TrimSpace(body)
	if body == "" || body[0] != '{' {
		return false
	}
	var msg jsonrpcMessage
	if err := json.Unmarshal([]byte(body), &msg); err != nil {
		return false
	}
	return msg.Error != nil
}

func streamableInitialize(ctx context.Context, client *http.Client, srv *mcp.Server) (sessionID, negotiated string, err error) {
	params := initializeParams{
		ProtocolVersion: offeredProtocolVersion,
		Capabilities:    map[string]any{},
		ClientInfo:      clientInfo{Name: "cc-switch-cli", Version: version.Version},
	}
	reqBody, err := encodeRequest(1, "initialize", params)
	if err != nil {
		return "", "", err
	}
	msg, sid, status, body, err := doStreamablePOST(ctx, client, srv, "", "", reqBody, true)
	if err != nil {
		return "", "", err
	}
	if status == 401 {
		return "", "", &httpProbeError{status: status, body: body, auth: true, msg: "authentication required"}
	}
	if status == 403 {
		return "", "", &httpProbeError{status: status, body: body, auth: true, msg: "forbidden (check token scopes)"}
	}
	if status < 200 || status >= 300 {
		return "", "", &httpProbeError{
			status: status,
			body:   body,
			msg:    fmt.Sprintf("initialize HTTP %d: %s", status, truncate(body, 200)),
		}
	}
	if msg == nil {
		return "", "", fmt.Errorf("initialize: empty response")
	}
	if msg.Error != nil {
		return "", "", fmt.Errorf("%s", classifyInitializeError(msg.Error))
	}
	var res initializeResult
	if err := json.Unmarshal(msg.Result, &res); err != nil {
		return "", "", fmt.Errorf("initialize result: %w", err)
	}
	return sid, res.ProtocolVersion, nil
}

func streamableNotify(ctx context.Context, client *http.Client, srv *mcp.Server, sessionID, negotiated string) error {
	body, err := encodeNotification("notifications/initialized", map[string]any{})
	if err != nil {
		return err
	}
	_, _, status, respBody, err := doStreamablePOST(ctx, client, srv, sessionID, negotiated, body, false)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("HTTP %d: %s", status, truncate(respBody, 200))
	}
	return nil
}

func streamableCall(ctx context.Context, client *http.Client, srv *mcp.Server, sessionID, negotiated string, id int, method string, params any) (*jsonrpcMessage, error) {
	reqBody, err := encodeRequest(id, method, params)
	if err != nil {
		return nil, err
	}
	msg, newSID, status, body, err := doStreamablePOST(ctx, client, srv, sessionID, negotiated, reqBody, true)
	if err != nil {
		return nil, err
	}
	// Session died: re-initialize once and retry.
	if status == 404 && sessionID != "" {
		freshSID, freshVer, initErr := streamableInitialize(ctx, client, srv)
		if initErr != nil {
			return nil, fmt.Errorf("session expired and re-initialize failed: %w", initErr)
		}
		if err := streamableNotify(ctx, client, srv, freshSID, freshVer); err != nil {
			return nil, fmt.Errorf("session re-init notify: %w", err)
		}
		msg, _, status, body, err = doStreamablePOST(ctx, client, srv, freshSID, freshVer, reqBody, true)
		if err != nil {
			return nil, err
		}
		_ = newSID
	}
	if status == 401 {
		return nil, &httpProbeError{status: status, body: body, auth: true, msg: "authentication required"}
	}
	if status == 403 {
		return nil, &httpProbeError{status: status, body: body, auth: true, msg: "forbidden (check token scopes)"}
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", status, truncate(body, 200))
	}
	if msg == nil {
		return nil, fmt.Errorf("empty response")
	}
	return msg, nil
}

// doStreamablePOST posts a JSON-RPC message. expectResponse controls whether
// we try to parse a JSON-RPC response (false for notifications).
func doStreamablePOST(ctx context.Context, client *http.Client, srv *mcp.Server, sessionID, negotiated string, body []byte, expectResponse bool) (*jsonrpcMessage, string, int, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL, bytes.NewReader(body))
	if err != nil {
		return nil, "", 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Accept-Encoding", "identity")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	if negotiated != "" {
		req.Header.Set("MCP-Protocol-Version", negotiated)
	}
	for k, v := range srv.Headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", 0, "", err
	}
	defer resp.Body.Close()

	sid := resp.Header.Get("Mcp-Session-Id")
	if sid == "" {
		sid = resp.Header.Get("MCP-Session-Id")
	}
	if sid == "" {
		sid = sessionID
	}

	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if !expectResponse {
		// Drain body but do not parse.
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, sid, resp.StatusCode, string(b), nil
	}

	if strings.HasPrefix(ct, "text/event-stream") {
		msg, err := readSSEResponse(ctx, resp.Body, extractRequestID(body))
		if err != nil {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 0)) // already consumed
			_ = b
			return nil, sid, resp.StatusCode, "", err
		}
		return msg, sid, resp.StatusCode, "", nil
	}

	// Default: treat as a single JSON response (even without a Content-Type).
	raw, err := io.ReadAll(io.LimitReader(resp.Body, scannerMaxToken))
	if err != nil {
		return nil, sid, resp.StatusCode, "", err
	}
	bodyStr := string(raw)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, sid, resp.StatusCode, bodyStr, nil
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, sid, resp.StatusCode, bodyStr, nil
	}
	var msg jsonrpcMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil, sid, resp.StatusCode, bodyStr, fmt.Errorf("decode response: %w", err)
	}
	return &msg, sid, resp.StatusCode, bodyStr, nil
}

func extractRequestID(body []byte) int {
	var msg jsonrpcMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return 0
	}
	var n int
	if err := json.Unmarshal(msg.ID, &n); err == nil {
		return n
	}
	return 0
}

// --- legacy HTTP+SSE ---

func probeLegacySSE(ctx context.Context, e mcp.Entry, opts Options, r *Result) {
	srv := e.Server
	client := &http.Client{Timeout: 0}

	endpoint, events, cancel, err := openLegacySSE(ctx, client, srv)
	if err != nil {
		r.Status = StatusFail
		r.Error = err.Error()
		return
	}
	defer cancel()

	// initialize
	params := initializeParams{
		ProtocolVersion: offeredProtocolVersion,
		Capabilities:    map[string]any{},
		ClientInfo:      clientInfo{Name: "cc-switch-cli", Version: version.Version},
	}
	initReq, err := encodeRequest(1, "initialize", params)
	if err != nil {
		r.Status = StatusFail
		r.Error = err.Error()
		return
	}
	if err := legacyPOST(ctx, client, endpoint, srv, initReq); err != nil {
		r.Status = StatusFail
		r.Error = err.Error()
		return
	}
	initMsg, err := events.waitID(ctx, 1)
	if err != nil {
		r.Status = StatusFail
		r.Error = fmt.Sprintf("initialize: %v", err)
		return
	}
	if initMsg.Error != nil {
		r.Status = StatusFail
		r.Error = classifyInitializeError(initMsg.Error)
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

	// notifications/initialized
	notif, err := encodeNotification("notifications/initialized", map[string]any{})
	if err != nil {
		r.Status = StatusFail
		r.Error = err.Error()
		return
	}
	if err := legacyPOST(ctx, client, endpoint, srv, notif); err != nil {
		r.Status = StatusFail
		r.Error = fmt.Sprintf("notifications/initialized: %v", err)
		return
	}

	r.Status = StatusOK
	if !opts.WithTools {
		return
	}

	tools, err := listToolsVia(ctx, func(id int, method string, params any) (*jsonrpcMessage, error) {
		req, err := encodeRequest(id, method, params)
		if err != nil {
			return nil, err
		}
		if err := legacyPOST(ctx, client, endpoint, srv, req); err != nil {
			return nil, err
		}
		return events.waitID(ctx, id)
	})
	if err != nil {
		r.Error = fmt.Sprintf("tools/list: %v", err)
		r.ToolCount = -1
		return
	}
	r.Tools = tools
	r.ToolCount = len(tools)
}

func openLegacySSE(ctx context.Context, client *http.Client, srv *mcp.Server) (endpoint string, events *sseEventStream, cancel func(), err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if err != nil {
		return "", nil, nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Accept-Encoding", "identity")
	for k, v := range srv.Headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, nil, err
	}
	if resp.StatusCode == 401 {
		resp.Body.Close()
		return "", nil, nil, &httpProbeError{status: 401, auth: true, msg: "authentication required"}
	}
	if resp.StatusCode == 403 {
		resp.Body.Close()
		return "", nil, nil, &httpProbeError{status: 403, auth: true, msg: "forbidden (check token scopes)"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		return "", nil, nil, fmt.Errorf("SSE GET HTTP %d: %s", resp.StatusCode, truncate(string(b), 200))
	}

	stream := newSSEEventStream(resp.Body)
	// First event must be "endpoint".
	ev, err := stream.nextEvent(ctx)
	if err != nil {
		resp.Body.Close()
		return "", nil, nil, fmt.Errorf("waiting for endpoint event: %w", err)
	}
	// Accept event:endpoint, or a bare data frame (empty event name) that
	// carries the path — some servers omit the event field.
	if ev.event != "" && ev.event != "endpoint" {
		resp.Body.Close()
		return "", nil, nil, fmt.Errorf("expected endpoint event, got %q", ev.event)
	}
	ep, err := resolveEndpoint(srv.URL, strings.TrimSpace(ev.data))
	if err != nil {
		resp.Body.Close()
		return "", nil, nil, err
	}
	cancel = func() { resp.Body.Close() }
	// Continue reading message events in the background.
	stream.startBackground(ctx)
	return ep, stream, cancel, nil
}

func resolveEndpoint(sseURL, data string) (string, error) {
	if data == "" {
		return "", fmt.Errorf("empty endpoint data")
	}
	base, err := url.Parse(sseURL)
	if err != nil {
		return "", fmt.Errorf("parse SSE url: %w", err)
	}
	ref, err := url.Parse(data)
	if err != nil {
		return "", fmt.Errorf("parse endpoint: %w", err)
	}
	resolved := base.ResolveReference(ref)
	if resolved.Scheme != base.Scheme || resolved.Host != base.Host {
		return "", fmt.Errorf("endpoint origin %s://%s differs from SSE origin %s://%s",
			resolved.Scheme, resolved.Host, base.Scheme, base.Host)
	}
	return resolved.String(), nil
}

func legacyPOST(ctx context.Context, client *http.Client, endpoint string, srv *mcp.Server, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range srv.Headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("POST HTTP %d: %s", resp.StatusCode, truncate(string(b), 200))
	}
	return nil
}

// --- SSE parsing ---

type sseEvent struct {
	event string
	data  string
}

// readSSEResponse consumes an SSE stream until a JSON-RPC message whose id
// matches wantID arrives (or the stream ends).
func readSSEResponse(ctx context.Context, r io.Reader, wantID int) (*jsonrpcMessage, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), scannerMaxToken)
	var (
		eventType string
		dataLines []string
	)
	flush := func() (*jsonrpcMessage, bool, error) {
		if len(dataLines) == 0 && eventType == "" {
			return nil, false, nil
		}
		data := strings.Join(dataLines, "\n")
		et := eventType
		eventType = ""
		dataLines = nil
		if et != "" && et != "message" {
			return nil, false, nil
		}
		data = strings.TrimSpace(data)
		if data == "" || data[0] != '{' {
			return nil, false, nil
		}
		var msg jsonrpcMessage
		if err := json.Unmarshal([]byte(data), &msg); err != nil {
			return nil, false, nil
		}
		if wantID != 0 && !idMatches(msg.ID, wantID) {
			return nil, false, nil
		}
		return &msg, true, nil
	}
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		if !sc.Scan() {
			if err := sc.Err(); err != nil {
				return nil, err
			}
			// Stream closed; try a final flush.
			if msg, ok, _ := flush(); ok {
				return msg, nil
			}
			return nil, fmt.Errorf("SSE stream ended before response")
		}
		line := sc.Text()
		if line == "" {
			if msg, ok, err := flush(); err != nil {
				return nil, err
			} else if ok {
				return msg, nil
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue // comment
		}
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data:"))
			// Spec allows "data:" with optional leading space.
			if len(dataLines) > 0 {
				dataLines[len(dataLines)-1] = strings.TrimPrefix(dataLines[len(dataLines)-1], " ")
			}
			continue
		}
		// Ignore id: / retry: and unknown fields.
	}
}

// sseEventStream is used by the legacy transport: one long-lived GET that
// delivers endpoint then message events.
type sseEventStream struct {
	sc      *bufio.Scanner
	pending chan sseEvent
	errCh   chan error
	closed  chan struct{}
}

func newSSEEventStream(r io.Reader) *sseEventStream {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), scannerMaxToken)
	return &sseEventStream{
		sc:      sc,
		pending: make(chan sseEvent, 16),
		errCh:   make(chan error, 1),
		closed:  make(chan struct{}),
	}
}

func (s *sseEventStream) nextEvent(ctx context.Context) (sseEvent, error) {
	var (
		eventType string
		dataLines []string
	)
	for {
		if !s.sc.Scan() {
			if err := s.sc.Err(); err != nil {
				return sseEvent{}, err
			}
			return sseEvent{}, io.EOF
		}
		select {
		case <-ctx.Done():
			return sseEvent{}, ctx.Err()
		default:
		}
		line := s.sc.Text()
		if line == "" {
			if len(dataLines) == 0 && eventType == "" {
				continue
			}
			data := strings.Join(dataLines, "\n")
			ev := sseEvent{event: eventType, data: data}
			return ev, nil
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			d := strings.TrimPrefix(line, "data:")
			d = strings.TrimPrefix(d, " ")
			dataLines = append(dataLines, d)
			continue
		}
	}
}

func (s *sseEventStream) startBackground(ctx context.Context) {
	go func() {
		defer close(s.closed)
		for {
			ev, err := s.nextEvent(ctx)
			if err != nil {
				select {
				case s.errCh <- err:
				default:
				}
				return
			}
			select {
			case s.pending <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (s *sseEventStream) waitID(ctx context.Context, wantID int) (*jsonrpcMessage, error) {
	deadline := time.Now().Add(30 * time.Second)
	if dl, ok := ctx.Deadline(); ok {
		deadline = dl
	}
	for {
		remain := time.Until(deadline)
		if remain <= 0 {
			return nil, context.DeadlineExceeded
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case err := <-s.errCh:
			return nil, err
		case ev := <-s.pending:
			if ev.event != "" && ev.event != "message" {
				continue
			}
			data := strings.TrimSpace(ev.data)
			if data == "" || data[0] != '{' {
				continue
			}
			var msg jsonrpcMessage
			if err := json.Unmarshal([]byte(data), &msg); err != nil {
				continue
			}
			if idMatches(msg.ID, wantID) {
				return &msg, nil
			}
		case <-time.After(remain):
			return nil, context.DeadlineExceeded
		}
	}
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
