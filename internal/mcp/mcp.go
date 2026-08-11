// Package mcp models the MCP server entries Claude Code keeps in
// ~/.claude.json (user and local scopes) and .mcp.json (project scope) and
// edits those files losslessly. Storage layout details live in store.go.
package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"cc-switch-cli/internal/ui"
)

// Scope identifies where an MCP server entry lives.
type Scope string

const (
	// ScopeUser is ~/.claude.json top-level mcpServers.
	ScopeUser Scope = "user"
	// ScopeLocal is ~/.claude.json projects.<dir>.mcpServers.
	ScopeLocal Scope = "local"
	// ScopeProject is <dir>/.mcp.json.
	ScopeProject Scope = "project"
)

// AllScopes in Claude Code precedence order (highest first).
var AllScopes = []Scope{ScopeLocal, ScopeProject, ScopeUser}

// ParseScope accepts exactly user, local or project.
func ParseScope(s string) (Scope, error) {
	switch sc := Scope(strings.ToLower(strings.TrimSpace(s))); sc {
	case ScopeUser, ScopeLocal, ScopeProject:
		return sc, nil
	}
	return "", fmt.Errorf("unknown scope %q (want user, local or project)", s)
}

func (s Scope) String() string { return string(s) }

// Transport classifies how a server is reached.
type Transport string

const (
	TransportStdio   Transport = "stdio"
	TransportHTTP    Transport = "http"
	TransportSSE     Transport = "sse"
	TransportWS      Transport = "ws"
	TransportInvalid Transport = "invalid"
)

func (t Transport) String() string { return string(t) }

// Server is one MCP server entry. Typed fields cover what cc-switch-cli
// understands; every other key (timeout, alwaysLoad, headersHelper, oauth...)
// survives verbatim through Rest.
type Server struct {
	Type    string
	Command string
	Args    []string
	Env     map[string]string
	URL     string
	Headers map[string]string
	Rest    map[string]json.RawMessage
}

// Typed JSON keys consumed by Server; anything else is kept in Rest.
var typedKeys = map[string]struct{}{
	"type": {}, "command": {}, "args": {}, "env": {}, "url": {}, "headers": {},
}

// MarshalJSON emits typed keys in canonical order followed by the sorted Rest
// keys, matching the lossless round-trip pattern used by model.Model.
func (s *Server) MarshalJSON() ([]byte, error) {
	buf := bytes.NewBufferString("{")
	first := true
	writePair := func(key string, valJSON []byte) error {
		if !first {
			buf.WriteByte(',')
		}
		first = false
		keyJSON, err := json.Marshal(key)
		if err != nil {
			return err
		}
		buf.Write(keyJSON)
		buf.WriteByte(':')
		buf.Write(valJSON)
		return nil
	}
	writeString := func(key, val string) error {
		if val == "" {
			return nil
		}
		valJSON, err := json.Marshal(val)
		if err != nil {
			return err
		}
		return writePair(key, valJSON)
	}
	writeStringMap := func(key string, m map[string]string) error {
		if len(m) == 0 {
			return nil
		}
		inner := bytes.NewBufferString("{")
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			if i > 0 {
				inner.WriteByte(',')
			}
			kJSON, err := json.Marshal(k)
			if err != nil {
				return err
			}
			vJSON, err := json.Marshal(m[k])
			if err != nil {
				return err
			}
			inner.Write(kJSON)
			inner.WriteByte(':')
			inner.Write(vJSON)
		}
		inner.WriteByte('}')
		return writePair(key, inner.Bytes())
	}

	if err := writeString("type", s.Type); err != nil {
		return nil, err
	}
	if err := writeString("command", s.Command); err != nil {
		return nil, err
	}
	if len(s.Args) > 0 {
		valJSON, err := json.Marshal(s.Args)
		if err != nil {
			return nil, err
		}
		if err := writePair("args", valJSON); err != nil {
			return nil, err
		}
	}
	if err := writeStringMap("env", s.Env); err != nil {
		return nil, err
	}
	if err := writeString("url", s.URL); err != nil {
		return nil, err
	}
	if err := writeStringMap("headers", s.Headers); err != nil {
		return nil, err
	}
	if len(s.Rest) > 0 {
		keys := make([]string, 0, len(s.Rest))
		for k := range s.Rest {
			if _, typed := typedKeys[k]; !typed {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			if err := writePair(k, s.Rest[k]); err != nil {
				return nil, err
			}
		}
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// UnmarshalJSON splits typed fields from the rest of the entry.
func (s *Server) UnmarshalJSON(data []byte) error {
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("mcp server entry must be a JSON object: %w", err)
	}
	*s = Server{}

	str := func(key string, dst *string) error {
		v, ok := raw[key]
		if !ok {
			return nil
		}
		var x string
		if err := json.Unmarshal(v, &x); err != nil {
			return fmt.Errorf("mcp field %q must be a string", key)
		}
		*dst = x
		delete(raw, key)
		return nil
	}
	strList := func(key string, dst *[]string) error {
		v, ok := raw[key]
		if !ok {
			return nil
		}
		var xs []string
		if err := json.Unmarshal(v, &xs); err != nil {
			return fmt.Errorf("mcp field %q must be an array of strings", key)
		}
		*dst = xs
		delete(raw, key)
		return nil
	}
	strMap := func(key string, dst *map[string]string) error {
		v, ok := raw[key]
		if !ok {
			return nil
		}
		m := map[string]string{}
		if err := json.Unmarshal(v, &m); err != nil {
			return fmt.Errorf("mcp field %q must be an object of strings", key)
		}
		*dst = m
		delete(raw, key)
		return nil
	}

	if err := str("type", &s.Type); err != nil {
		return err
	}
	if err := str("command", &s.Command); err != nil {
		return err
	}
	if err := strList("args", &s.Args); err != nil {
		return err
	}
	if err := strMap("env", &s.Env); err != nil {
		return err
	}
	if err := str("url", &s.URL); err != nil {
		return err
	}
	if err := strMap("headers", &s.Headers); err != nil {
		return err
	}
	if len(raw) > 0 {
		s.Rest = raw
	}
	return nil
}

// ParseServer builds a Server from raw JSON.
func ParseServer(data []byte) (*Server, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New("empty server entry")
	}
	var s Server
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Transport derives the transport from the entry. A url without any type is
// invalid because Claude Code reads typeless entries as stdio.
func (s *Server) Transport() Transport {
	if s == nil {
		return TransportInvalid
	}
	switch strings.ToLower(s.Type) {
	case "", "stdio":
		if s.Command != "" && s.URL == "" {
			return TransportStdio
		}
		return TransportInvalid
	case "http", "streamable-http":
		return TransportHTTP
	case "sse":
		return TransportSSE
	case "ws":
		return TransportWS
	}
	return TransportInvalid
}

// Validate checks structural sanity. It mirrors the rules Claude Code applies
// when loading servers.
func (s *Server) Validate() error {
	if s == nil {
		return errors.New("empty server entry")
	}
	switch s.Transport() {
	case TransportStdio:
		if strings.TrimSpace(s.Command) == "" {
			return errors.New("stdio server needs a non-empty command")
		}
	case TransportHTTP, TransportSSE:
		if err := checkURL(s.URL, "http", "https"); err != nil {
			return fmt.Errorf("%s server: %w", s.Transport(), err)
		}
	case TransportWS:
		if err := checkURL(s.URL, "ws", "wss", "http", "https"); err != nil {
			return fmt.Errorf("ws server: %w", err)
		}
	default:
		if s.URL != "" && s.Type == "" {
			return errors.New(`entry has a "url" but no "type"; add "type": "http" (or "sse" / "ws")`)
		}
		return errors.New("server entry needs either a command (stdio) or a type with url")
	}
	if s.Type != "" && strings.ToLower(s.Type) != "stdio" && s.Command != "" {
		return errors.New("entry mixes a url-based type with a stdio command")
	}
	return nil
}

func checkURL(raw string, schemes ...string) error {
	if strings.TrimSpace(raw) == "" {
		return errors.New("missing url")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("invalid url %q", raw)
	}
	for _, sc := range schemes {
		if u.Scheme == sc {
			return nil
		}
	}
	return fmt.Errorf("url %q must use one of %s", raw, strings.Join(schemes, ", "))
}

// Indent renders the entry as pretty JSON with two-space indent and a
// trailing newline.
func (s *Server) Indent() ([]byte, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "", "  "); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

// Entry is a server plus where it was found.
type Entry struct {
	Name   string
	Scope  Scope
	Server *Server
}

var (
	reName         = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	reSecretHeader = regexp.MustCompile(`(?i)(auth|token|key|secret|cookie)`)
	reSecretEnv    = regexp.MustCompile(`(?i)(pass|token|key|secret)`)
)

// Reserved server names rejected by Claude Code.
var reservedNames = map[string]struct{}{
	"workspace":        {},
	"claude-in-chrome": {},
	"computer-use":     {},
	"claude_preview":   {},
	"claude_browser":   {},
}

// ValidateName enforces the character set claude mcp add applies and rejects
// reserved names.
func ValidateName(name string) error {
	if !reName.MatchString(name) {
		return fmt.Errorf("invalid server name %q: use letters, digits, hyphens and underscores (max 64 chars)", name)
	}
	if _, ok := reservedNames[strings.ToLower(name)]; ok {
		return fmt.Errorf("server name %q is reserved by Claude Code", name)
	}
	return nil
}

// Masked returns a copy safe for display: header and env values whose names
// look like credentials are masked and passwords embedded in URLs are
// redacted. Rest is shared with the receiver; treat the result as read-only.
func (s *Server) Masked() *Server {
	if s == nil {
		return nil
	}
	cp := *s
	if len(s.Args) > 0 {
		cp.Args = append([]string(nil), s.Args...)
		for i, a := range cp.Args {
			cp.Args[i] = redactURLPassword(a)
		}
	}
	cp.Env = maskStringMap(s.Env, reSecretEnv)
	cp.Headers = maskStringMap(s.Headers, reSecretHeader)
	cp.URL = redactURLPassword(s.URL)
	return &cp
}

func maskStringMap(m map[string]string, re *regexp.Regexp) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		if re.MatchString(k) {
			out[k] = ui.MaskToken(v)
		} else {
			out[k] = v
		}
	}
	return out
}

// redactURLPassword replaces the password component of a URL-ish string.
// Values that are not URLs, or carry no password, come back unchanged. The
// mask is spliced into the original text so it is not percent-escaped.
func redactURLPassword(s string) string {
	if !strings.Contains(s, "://") {
		return s
	}
	u, err := url.Parse(s)
	if err != nil || u.User == nil {
		return s
	}
	if _, has := u.User.Password(); !has {
		return s
	}
	at := strings.LastIndex(s, "@")
	userinfo := s[:at]
	colon := strings.Index(userinfo, ":")
	if colon < 0 {
		return s
	}
	return userinfo[:colon+1] + "****" + s[at:]
}
