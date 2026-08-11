package mcptest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

// Protocol versions offered / accepted by the probe. We speak the last
// handshake-based revision and accept the other widely-deployed ones.
const (
	offeredProtocolVersion = "2025-11-25"
)

var acceptedProtocolVersions = map[string]struct{}{
	"2024-11-05": {},
	"2025-03-26": {},
	"2025-06-18": {},
	"2025-11-25": {},
}

// jsonrpcMessage is a lenient JSON-RPC 2.0 envelope. id may be a number,
// string, or null; result and error are kept as RawMessage so callers can
// decode them into typed shapes.
type jsonrpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *jsonrpcError) Error() string {
	if e == nil {
		return ""
	}
	if len(e.Data) > 0 {
		return fmt.Sprintf("json-rpc %d: %s (%s)", e.Code, e.Message, string(e.Data))
	}
	return fmt.Sprintf("json-rpc %d: %s", e.Code, e.Message)
}

func encodeRequest(id int, method string, params any) ([]byte, error) {
	var paramsRaw json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		paramsRaw = b
	}
	msg := jsonrpcMessage{
		JSONRPC: "2.0",
		ID:      json.RawMessage(strconv.AppendInt(nil, int64(id), 10)),
		Method:  method,
		Params:  paramsRaw,
	}
	return json.Marshal(msg)
}

func encodeNotification(method string, params any) ([]byte, error) {
	var paramsRaw json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		paramsRaw = b
	}
	msg := jsonrpcMessage{
		JSONRPC: "2.0",
		Method:  method,
		Params:  paramsRaw,
	}
	return json.Marshal(msg)
}

// idMatches reports whether a response id equals the numeric id we sent.
// Accepts both number and string forms; ignores null/absent.
func idMatches(raw json.RawMessage, want int) bool {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return false
	}
	var n int
	if err := json.Unmarshal(raw, &n); err == nil {
		return n == want
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s == strconv.Itoa(want)
	}
	return false
}

type initializeParams struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ClientInfo      clientInfo     `json:"clientInfo"`
}

type clientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type initializeResult struct {
	ProtocolVersion string          `json:"protocolVersion"`
	Capabilities    json.RawMessage `json:"capabilities"`
	ServerInfo      json.RawMessage `json:"serverInfo"`
}

type toolsListResult struct {
	Tools      []toolEntry `json:"tools"`
	NextCursor string      `json:"nextCursor"`
}

type toolEntry struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// classifyInitializeError turns a JSON-RPC error on initialize into a human
// message. Modern-era (2026-07-28) servers have no initialize and answer with
// -32601 or -32022.
func classifyInitializeError(err *jsonrpcError) string {
	if err == nil {
		return "initialize failed"
	}
	switch err.Code {
	case -32601, -32022:
		supported := extractSupported(err.Data)
		if supported != "" {
			return fmt.Sprintf("server requires MCP %s (no initialize handshake)", supported)
		}
		return "server requires a newer MCP era (no initialize handshake)"
	}
	return err.Error()
}

func extractSupported(data json.RawMessage) string {
	if len(data) == 0 {
		return ""
	}
	var obj struct {
		Supported []string `json:"supported"`
	}
	if err := json.Unmarshal(data, &obj); err == nil && len(obj.Supported) > 0 {
		return obj.Supported[0]
	}
	// Some servers put the version list at the top level of data as an array.
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil && len(arr) > 0 {
		return arr[0]
	}
	return ""
}

func isAcceptedVersion(v string) bool {
	_, ok := acceptedProtocolVersions[v]
	return ok
}
