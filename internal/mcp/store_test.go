package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// sandboxHome points HOME/USERPROFILE at a temp dir so config.Home() and
// ClaudeJSONPath resolve inside it. CLAUDE_CONFIG_DIR is cleared unless a
// test sets it explicitly.
func sandboxHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	return dir
}

func writeClaudeJSON(t *testing.T, home, content string) string {
	t.Helper()
	path := filepath.Join(home, ".claude.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func readClaudeDoc(t *testing.T, home string) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".claude.json"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("reparse: %v", err)
	}
	return doc
}

func stdioServer(cmd string, args ...string) *Server {
	return &Server{Command: cmd, Args: args}
}

func httpServer(url string) *Server {
	return &Server{Type: "http", URL: url}
}

func TestListMissingFileIsEmpty(t *testing.T) {
	sandboxHome(t)
	for _, sc := range AllScopes {
		entries, err := List(sc, t.TempDir())
		if err != nil || len(entries) != 0 {
			t.Fatalf("scope %s: want empty list without error, got %v, %v", sc, entries, err)
		}
	}
}

func TestUserScopeCRUD(t *testing.T) {
	home := sandboxHome(t)
	writeClaudeJSON(t, home, `{
  "numStartups": 42,
  "oauthAccount": {"email": "x@example.com"},
  "projects": {"C:/Users/x": {"hasTrustDialogAccepted": true}},
  "mcpServers": {"fetch": {"command": "mcp-server-fetch", "args": []}}
}`)

	entries, err := List(ScopeUser, "")
	if err != nil || len(entries) != 1 || entries[0].Name != "fetch" {
		t.Fatalf("list: %v, %v", entries, err)
	}
	if entries[0].Server.Transport() != TransportStdio {
		t.Fatalf("want stdio transport, got %s", entries[0].Server.Transport())
	}

	if err := Set(ScopeUser, "", "my-http", httpServer("https://mcp.example.com/mcp")); err != nil {
		t.Fatalf("set: %v", err)
	}
	srv, err := Get(ScopeUser, "", "my-http")
	if err != nil || srv.URL != "https://mcp.example.com/mcp" {
		t.Fatalf("get: %v, %v", srv, err)
	}

	// Surrounding state must survive byte-for-byte where we didn't touch it.
	doc := readClaudeDoc(t, home)
	if string(doc["numStartups"]) != "42" {
		t.Fatalf("numStartups lost: %s", doc["numStartups"])
	}
	if !strings.Contains(string(doc["oauthAccount"]), "x@example.com") {
		t.Fatalf("oauthAccount lost: %s", doc["oauthAccount"])
	}
	projects := rawObject(doc, "projects")
	if !strings.Contains(string(projects["C:/Users/x"]), "hasTrustDialogAccepted") {
		t.Fatalf("project entry lost its fields: %s", projects["C:/Users/x"])
	}

	if err := Remove(ScopeUser, "", "my-http"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := Get(ScopeUser, "", "my-http"); err == nil {
		t.Fatal("server should be gone")
	}
	if err := Remove(ScopeUser, "", "my-http"); err == nil {
		t.Fatal("second remove should fail")
	}
}

func TestLocalScopeReusesExistingKeyCaseInsensitive(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("drive-letter keys are Windows-only")
	}
	home := sandboxHome(t)
	// Stored key uses a lower-case drive letter; lookups must still match it.
	writeClaudeJSON(t, home, `{
  "projects": {
    "e:/work/demo": {"mcpServers": {"db": {"command": "db-mcp"}}},
    "C:/Other": {"lastCost": 1.5}
  }
}`)

	entries, err := List(ScopeLocal, "E:/work/demo")
	if err != nil || len(entries) != 1 || entries[0].Name != "db" {
		t.Fatalf("local list: %v, %v", entries, err)
	}
	if err := Set(ScopeLocal, "E:/work/demo", "extra", httpServer("https://x/mcp")); err != nil {
		t.Fatalf("local set: %v", err)
	}
	doc := readClaudeDoc(t, home)
	projects := rawObject(doc, "projects")
	if _, ok := projects["e:/work/demo"]; !ok {
		t.Fatalf("write created a new key instead of reusing the existing one: %v", projects)
	}
	if len(projects) != 2 {
		t.Fatalf("unexpected key count: %v", projects)
	}
}

func TestLocalScopeCreatesNormalizedKey(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("drive-letter normalization is Windows-only")
	}
	home := sandboxHome(t)
	writeClaudeJSON(t, home, `{"projects": {}}`)

	dir := "e:/work/newproj"
	if err := Set(ScopeLocal, dir, "srv", stdioServer("cmd")); err != nil {
		t.Fatalf("set: %v", err)
	}
	doc := readClaudeDoc(t, home)
	projects := rawObject(doc, "projects")
	if _, ok := projects["E:/work/newproj"]; !ok {
		t.Fatalf("normalized key not written: %v", projects)
	}
}

func TestProjectScopeFile(t *testing.T) {
	sandboxHome(t)
	dir := t.TempDir()

	if err := Set(ScopeProject, dir, "srv-a", stdioServer("a-mcp")); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := Set(ScopeProject, dir, "srv-b", httpServer("https://x/mcp")); err != nil {
		t.Fatalf("set: %v", err)
	}
	entries, err := List(ScopeProject, dir)
	if err != nil || len(entries) != 2 {
		t.Fatalf("list: %v, %v", entries, err)
	}
	if entries[0].Name != "srv-a" || entries[1].Name != "srv-b" {
		t.Fatalf("want sorted names, got %v %v", entries[0].Name, entries[1].Name)
	}

	if err := Remove(ScopeProject, dir, "srv-a"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	if err != nil {
		t.Fatalf("file gone after removing one of two: %v", err)
	}
	if !strings.Contains(string(data), "srv-b") {
		t.Fatalf("srv-b lost: %s", data)
	}
	if err := Remove(ScopeProject, dir, "srv-b"); err != nil {
		t.Fatalf("remove last: %v", err)
	}
	data, err = os.ReadFile(filepath.Join(dir, ".mcp.json"))
	if err != nil {
		t.Fatalf("file should remain after removing last entry: %v", err)
	}
	if !strings.Contains(string(data), "mcpServers") {
		t.Fatalf("empty mcpServers object expected: %s", data)
	}
}

func TestListAllPrecedenceOrder(t *testing.T) {
	home := sandboxHome(t)
	dir := t.TempDir()
	writeClaudeJSON(t, home, `{
  "mcpServers": {"shared": {"command": "user-srv"}, "user-only": {"command": "u"}},
  "projects": {}
}`)
	if err := Set(ScopeLocal, dir, "shared", stdioServer("local-srv")); err != nil {
		t.Fatalf("set local: %v", err)
	}
	if err := Set(ScopeProject, dir, "proj-only", stdioServer("p")); err != nil {
		t.Fatalf("set project: %v", err)
	}

	entries, err := ListAll(dir)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	var order []string
	for _, e := range entries {
		order = append(order, string(e.Scope)+"/"+e.Name)
	}
	want := []string{"local/shared", "project/proj-only", "user/shared", "user/user-only"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("order %v, want %v", order, want)
	}
}

func TestResolve(t *testing.T) {
	home := sandboxHome(t)
	dir := t.TempDir()
	writeClaudeJSON(t, home, `{"mcpServers": {"only-user": {"command": "x"}, "dup": {"command": "u"}}}`)
	if err := Set(ScopeLocal, dir, "dup", stdioServer("l")); err != nil {
		t.Fatalf("set: %v", err)
	}

	e, err := Resolve(dir, "only-user")
	if err != nil || e.Scope != ScopeUser {
		t.Fatalf("resolve single: %v, %v", e, err)
	}
	if _, err := Resolve(dir, "dup"); err == nil || !strings.Contains(err.Error(), "--scope") {
		t.Fatalf("ambiguous resolve should mention --scope: %v", err)
	}
	if _, err := Resolve(dir, "ghost"); err == nil {
		t.Fatal("missing resolve should fail")
	}
}

func TestClaudeConfigDirOverride(t *testing.T) {
	sandboxHome(t)
	alt := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", alt)

	// ~/.claude.json does not exist anywhere yet: must fail cleanly.
	if err := Set(ScopeUser, "", "cfg-dir-srv", stdioServer("x")); err == nil {
		t.Fatal("set without existing config should fail")
	}
	writeClaudeJSON(t, alt, `{"mcpServers": {}}`)
	if err := Set(ScopeUser, "", "cfg-dir-srv", stdioServer("x")); err != nil {
		t.Fatalf("set with CLAUDE_CONFIG_DIR: %v", err)
	}
	if _, err := os.Stat(filepath.Join(alt, ".claude.json")); err != nil {
		t.Fatalf("override location not used: %v", err)
	}
}

func TestSetCreatesBackup(t *testing.T) {
	home := sandboxHome(t)
	original := `{"mcpServers": {"a": {"command": "a-mcp"}}, "sentinel": true}`
	writeClaudeJSON(t, home, original)

	if err := Set(ScopeUser, "", "b", stdioServer("b-mcp")); err != nil {
		t.Fatalf("set: %v", err)
	}

	backupDir := filepath.Join(home, ".cc-switch-cli", "backups")
	dirEntries, err := os.ReadDir(backupDir)
	if err != nil || len(dirEntries) != 1 {
		t.Fatalf("want exactly one backup, got %v, %v", dirEntries, err)
	}
	data, err := os.ReadFile(filepath.Join(backupDir, dirEntries[0].Name()))
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(data) != original {
		t.Fatalf("backup does not hold the pre-mutation bytes")
	}
}

func TestSetRejectsBadInput(t *testing.T) {
	home := sandboxHome(t)
	writeClaudeJSON(t, home, `{"mcpServers": {}}`)

	if err := Set(ScopeUser, "", "bad name", stdioServer("x")); err == nil {
		t.Fatal("invalid name accepted")
	}
	if err := Set(ScopeUser, "", "workspace", stdioServer("x")); err == nil {
		t.Fatal("reserved name accepted")
	}
	if err := Set(ScopeUser, "", "ok", &Server{URL: "https://x/mcp"}); err == nil {
		t.Fatal("url-without-type accepted")
	}
	if err := Set(ScopeUser, "", "ok", nil); err == nil {
		t.Fatal("nil server accepted")
	}
}

func TestNormalizeProjectDir(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("drive-letter cases are Windows-only")
	}
	if got := NormalizeProjectDir(`e:\work\proj`); got != "E:/work/proj" {
		t.Fatalf("got %q", got)
	}
	if got := NormalizeProjectDir("E:/work/proj"); got != "E:/work/proj" {
		t.Fatalf("got %q", got)
	}
}
