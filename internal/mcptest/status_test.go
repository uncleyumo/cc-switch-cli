package mcptest

import (
	"testing"

	"cc-switch-cli/internal/mcp"
)

func sandboxHome(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
}

func TestStatusCacheRoundTrip(t *testing.T) {
	sandboxHome(t)

	sf, err := LoadStatus()
	if err != nil || sf != nil {
		t.Fatalf("missing file should yield nil,nil: %v %v", sf, err)
	}

	sf = &StatusFile{}
	sf.Upsert([]Result{
		{Name: "a", Scope: "user", Status: StatusOK, ToolCount: 3, TestedAt: "t1"},
		{Name: "b", Scope: "local", Status: StatusFail, Error: "x", ToolCount: -1, TestedAt: "t2"},
	})
	if err := SaveStatus(sf); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := LoadStatus()
	if err != nil || loaded == nil {
		t.Fatalf("load: %v %v", loaded, err)
	}
	if got := loaded.Results[CacheKey(mcp.ScopeUser, "a")]; got.Status != StatusOK || got.ToolCount != 3 {
		t.Fatalf("user/a = %+v", got)
	}
	if got := loaded.Results[CacheKey(mcp.ScopeLocal, "b")]; got.Status != StatusFail || got.Error != "x" {
		t.Fatalf("local/b = %+v", got)
	}

	// Upsert overwrites the same key.
	loaded.Upsert([]Result{{Name: "a", Scope: "user", Status: StatusFail, ToolCount: -1, TestedAt: "t3"}})
	if err := SaveStatus(loaded); err != nil {
		t.Fatalf("save2: %v", err)
	}
	loaded2, _ := LoadStatus()
	if got := loaded2.Results[CacheKey(mcp.ScopeUser, "a")]; got.Status != StatusFail {
		t.Fatalf("overwrite failed: %+v", got)
	}
}

func TestCacheKey(t *testing.T) {
	if got := CacheKey(mcp.ScopeUser, "fetch-mcp"); got != "user/fetch-mcp" {
		t.Fatalf("got %q", got)
	}
}
