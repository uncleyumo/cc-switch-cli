package transfer

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"cc-switch-cli/internal/config"
	"cc-switch-cli/internal/mcp"
	"cc-switch-cli/internal/model"
)

// sandboxHome points HOME/USERPROFILE at a temp dir so model and mcp IO
// resolve inside it. CLAUDE_CONFIG_DIR is cleared unless a test sets it.
func sandboxHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	return dir
}

func writeClaudeJSON(t *testing.T, home, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func seedFixtures(t *testing.T, home string) {
	t.Helper()
	writeClaudeJSON(t, home, `{"mcpServers": {}}`)
	saveModel(t, "anthropic", "sk-ant-alpha")
	saveModel(t, "proxy", "sk-ant-beta")
	setServer(t, "fetch", &mcp.Server{
		Command: "mcp-server-fetch",
		Args:    []string{"--quiet"},
		Env:     map[string]string{"FETCH_TIMEOUT": "30"},
	})
	setServer(t, "github", &mcp.Server{
		Type:    "http",
		URL:     "https://api.githubcopilot.com/mcp/",
		Headers: map[string]string{"Authorization": "Bearer ghp_x"},
	})
}

func saveModel(t *testing.T, name, token string) {
	t.Helper()
	m := &model.Model{
		Env: model.Env{
			AuthToken: token,
			BaseURL:   "https://api.anthropic.com",
			Model:     "claude-sonnet-4-6",
		},
		Rest: map[string]json.RawMessage{
			"model":          json.RawMessage(`"sonnet"`),
			"enabledPlugins": json.RawMessage(`{"git":true}`),
		},
	}
	if err := model.Save(name, m); err != nil {
		t.Fatalf("save model %s: %v", name, err)
	}
}

func setServer(t *testing.T, name string, srv *mcp.Server) {
	t.Helper()
	if err := mcp.Set(mcp.ScopeUser, "", name, srv); err != nil {
		t.Fatalf("set server %s: %v", name, err)
	}
}

// sameJSON compares two JSON documents by parsed value; Seal compacts the
// stored bytes, so whitespace differs after a round-trip.
func sameJSON(t *testing.T, a, b []byte) bool {
	t.Helper()
	var va, vb any
	if err := json.Unmarshal(a, &va); err != nil {
		t.Fatalf("unmarshal left side: %v", err)
	}
	if err := json.Unmarshal(b, &vb); err != nil {
		return false
	}
	return reflect.DeepEqual(va, vb)
}

func TestBuildPayload(t *testing.T) {
	home := sandboxHome(t)
	seedFixtures(t, home)

	raw, err := model.ReadRaw("anthropic")
	if err != nil {
		t.Fatalf("read model: %v", err)
	}
	p, err := BuildPayload([]string{"anthropic"}, []string{"github"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(p.Models) != 1 || len(p.MCPs) != 1 {
		t.Fatalf("unexpected payload size: %d models, %d mcps", len(p.Models), len(p.MCPs))
	}
	if !bytes.Equal(p.Models["anthropic"], raw) {
		t.Fatalf("model bytes differ from stored file")
	}
	var srv mcp.Server
	if err := json.Unmarshal(p.MCPs["github"], &srv); err != nil {
		t.Fatalf("mcp payload not a server: %v", err)
	}
	if srv.URL != "https://api.githubcopilot.com/mcp/" || srv.Headers["Authorization"] != "Bearer ghp_x" {
		t.Fatalf("unexpected server payload: %+v", srv)
	}

	// Empty selection lists leave the corresponding map nil.
	empty, err := BuildPayload(nil, nil)
	if err != nil {
		t.Fatalf("build empty: %v", err)
	}
	if empty.Models != nil || empty.MCPs != nil {
		t.Fatalf("want nil maps for empty selection, got %+v", empty)
	}
}

func TestBuildPayloadErrors(t *testing.T) {
	home := sandboxHome(t)
	seedFixtures(t, home)

	if _, err := BuildPayload([]string{"ghost"}, nil); err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("unknown model must be named in the error, got %v", err)
	}
	if _, err := BuildPayload(nil, []string{"ghost"}); err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("unknown mcp must be named in the error, got %v", err)
	}

	dir, err := config.ModelsDir()
	if err != nil {
		t.Fatalf("models dir: %v", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("not json"), 0o644); err != nil {
		t.Fatalf("write broken model: %v", err)
	}
	if _, err := BuildPayload([]string{"broken"}, nil); err == nil || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("malformed model must be named in the error, got %v", err)
	}
}

func TestExportImportEndToEnd(t *testing.T) {
	home := sandboxHome(t)
	seedFixtures(t, home)

	origModel, err := model.ReadRaw("anthropic")
	if err != nil {
		t.Fatalf("read original model: %v", err)
	}
	origFetch, err := mcp.Get(mcp.ScopeUser, "", "fetch")
	if err != nil {
		t.Fatalf("read original server: %v", err)
	}

	payload, err := BuildPayload([]string{"anthropic"}, []string{"fetch"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	env, err := Seal(payload, "transfer-secret-1")
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}

	// Wipe the home directory to simulate a fresh machine.
	if err := os.RemoveAll(home); err != nil {
		t.Fatalf("wipe home: %v", err)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("recreate home: %v", err)
	}

	checked, err := CheckEnvelope(data)
	if err != nil {
		t.Fatalf("check envelope: %v", err)
	}
	restored, err := Open(checked, "transfer-secret-1")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	rep, err := Apply(restored, ImportOptions{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !reflect.DeepEqual(rep.ModelsApplied, []string{"anthropic"}) ||
		!reflect.DeepEqual(rep.MCPsApplied, []string{"fetch"}) ||
		len(rep.ModelsSkipped) != 0 || len(rep.MCPsSkipped) != 0 {
		t.Fatalf("unexpected report: %+v", rep)
	}

	gotModel, err := model.ReadRaw("anthropic")
	if err != nil {
		t.Fatalf("model not restored: %v", err)
	}
	if !sameJSON(t, origModel, gotModel) {
		t.Fatalf("restored model differs from original:\n%s\n%s", origModel, gotModel)
	}
	if _, err := os.Stat(filepath.Join(home, ".cc-switch-cli", "models", "proxy.json")); !os.IsNotExist(err) {
		t.Fatalf("unselected model appeared after import: %v", err)
	}
	gotFetch, err := mcp.Get(mcp.ScopeUser, "", "fetch")
	if err != nil {
		t.Fatalf("server not restored: %v", err)
	}
	if !reflect.DeepEqual(gotFetch, origFetch) {
		t.Fatalf("restored server differs: %+v vs %+v", gotFetch, origFetch)
	}
	if _, err := mcp.Get(mcp.ScopeUser, "", "github"); err == nil {
		t.Fatal("unselected server appeared after import")
	}

	// Second run without force: everything exists and is skipped.
	rep, err = Apply(restored, ImportOptions{})
	if err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	if !reflect.DeepEqual(rep.ModelsSkipped, []string{"anthropic"}) ||
		!reflect.DeepEqual(rep.MCPsSkipped, []string{"fetch"}) ||
		len(rep.ModelsApplied) != 0 || len(rep.MCPsApplied) != 0 {
		t.Fatalf("expected everything skipped, got %+v", rep)
	}

	// Force re-applies over existing entries.
	rep, err = Apply(restored, ImportOptions{Force: true})
	if err != nil {
		t.Fatalf("force apply: %v", err)
	}
	if !reflect.DeepEqual(rep.ModelsApplied, []string{"anthropic"}) ||
		!reflect.DeepEqual(rep.MCPsApplied, []string{"fetch"}) {
		t.Fatalf("expected everything applied, got %+v", rep)
	}
}

func TestApplyKeepsExistingWithoutForce(t *testing.T) {
	home := sandboxHome(t)
	writeClaudeJSON(t, home, `{"mcpServers": {"fetch": {"command": "old-fetch"}}}`)
	saveModel(t, "anthropic", "sk-ant-old")
	beforeModel, err := model.ReadRaw("anthropic")
	if err != nil {
		t.Fatalf("read model: %v", err)
	}

	p := &Payload{
		Models: map[string]json.RawMessage{
			"anthropic": json.RawMessage(`{"env":{"ANTHROPIC_AUTH_TOKEN":"sk-ant-new"}}`),
		},
		MCPs: map[string]json.RawMessage{
			"fetch": json.RawMessage(`{"command":"new-fetch"}`),
		},
	}
	rep, err := Apply(p, ImportOptions{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(rep.ModelsApplied) != 0 || len(rep.MCPsApplied) != 0 {
		t.Fatalf("nothing should apply, got %+v", rep)
	}
	if !reflect.DeepEqual(rep.ModelsSkipped, []string{"anthropic"}) ||
		!reflect.DeepEqual(rep.MCPsSkipped, []string{"fetch"}) {
		t.Fatalf("unexpected skipped lists: %+v", rep)
	}
	afterModel, err := model.ReadRaw("anthropic")
	if err != nil {
		t.Fatalf("read model after: %v", err)
	}
	if !bytes.Equal(beforeModel, afterModel) {
		t.Fatalf("existing model was modified without force")
	}
	srv, err := mcp.Get(mcp.ScopeUser, "", "fetch")
	if err != nil {
		t.Fatalf("get server: %v", err)
	}
	if srv.Command != "old-fetch" {
		t.Fatalf("existing server was modified without force: %+v", srv)
	}

	rep, err = Apply(p, ImportOptions{Force: true})
	if err != nil {
		t.Fatalf("force apply: %v", err)
	}
	if !reflect.DeepEqual(rep.ModelsApplied, []string{"anthropic"}) ||
		!reflect.DeepEqual(rep.MCPsApplied, []string{"fetch"}) {
		t.Fatalf("unexpected force report: %+v", rep)
	}
	srv, err = mcp.Get(mcp.ScopeUser, "", "fetch")
	if err != nil {
		t.Fatalf("get server after force: %v", err)
	}
	if srv.Command != "new-fetch" {
		t.Fatalf("force did not overwrite the server: %+v", srv)
	}
}

// TestApplyWritesRawBytes verifies model entries land on disk exactly as
// stored in the payload.
func TestApplyWritesRawBytes(t *testing.T) {
	sandboxHome(t)
	raw := []byte("{\n  \"env\": {\n    \"ANTHROPIC_AUTH_TOKEN\": \"tok\"\n  }\n}\n")
	p := &Payload{Models: map[string]json.RawMessage{"raw-test": raw}}
	rep, err := Apply(p, ImportOptions{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !reflect.DeepEqual(rep.ModelsApplied, []string{"raw-test"}) {
		t.Fatalf("unexpected report: %+v", rep)
	}
	got, err := model.ReadRaw("raw-test")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !bytes.Equal(got, raw) {
		t.Fatalf("stored bytes differ:\nwant %q\ngot  %q", raw, got)
	}
}

func TestApplyRejectsBadEntries(t *testing.T) {
	sandboxHome(t)
	p := &Payload{Models: map[string]json.RawMessage{"bad name": json.RawMessage(`{}`)}}
	if _, err := Apply(p, ImportOptions{}); err == nil {
		t.Fatal("invalid model name accepted")
	}
	p = &Payload{Models: map[string]json.RawMessage{"ok": json.RawMessage(`42`)}}
	if _, err := Apply(p, ImportOptions{}); err == nil {
		t.Fatal("malformed model payload accepted")
	}
	p = &Payload{MCPs: map[string]json.RawMessage{"ok": json.RawMessage(`{"url":"https://x/mcp"}`)}}
	if _, err := Apply(p, ImportOptions{}); err == nil {
		t.Fatal("url-without-type server accepted")
	}
}

func TestParseSelection(t *testing.T) {
	available := []string{"alpha", "beta", "gamma"}
	cases := []struct {
		value   string
		want    []string
		wantErr string
	}{
		{"", nil, ""},
		{"all", []string{"alpha", "beta", "gamma"}, ""},
		{"none", []string{}, ""},
		{"gamma,alpha", []string{"alpha", "gamma"}, ""},
		{"beta,beta,alpha", []string{"alpha", "beta"}, ""},
		{" beta ", []string{"beta"}, ""},
		{"Alpha", nil, "Alpha"},
		{"alpha,ghost,zeta,ghost", nil, "ghost, zeta"},
		{"alpha,,beta", nil, "empty name"},
	}
	for _, tc := range cases {
		t.Run("value="+tc.value, func(t *testing.T) {
			got, err := ParseSelection(tc.value, available)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.want == nil {
				if got != nil {
					t.Fatalf("want nil, got %v", got)
				}
				return
			}
			if len(got) == 0 && len(tc.want) == 0 {
				if got == nil {
					t.Fatal("want non-nil empty slice")
				}
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}

	// "all" returns a copy: mutating it must not touch the source.
	got, err := ParseSelection("all", available)
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	got[0] = "mutated"
	if available[0] != "alpha" {
		t.Fatal("available slice was modified through the all-copy")
	}
}
