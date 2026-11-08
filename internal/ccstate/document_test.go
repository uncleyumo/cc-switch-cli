package ccstate

import (
	"encoding/json"
	"strings"
	"testing"
)

const sampleSettings = `{
  "env": {
    "ANTHROPIC_BASE_URL": "https://old.example.com",
    "KEEP_ME": "yes"
  },
  "permissions": {
    "deny": [
      "WebSearch",
      "WebFetch"
    ]
  },
  "model": "fable",
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "rtk hook claude"
          }
        ]
      }
    ]
  }
}
`

func TestParseRoundTripPreservesOrderAndFormatting(t *testing.T) {
	v, err := Parse([]byte(sampleSettings))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	out, err := Marshal(v, "  ")
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got := string(out) + "\n"
	if got != sampleSettings {
		t.Fatalf("round trip changed the document:\n--- got ---\n%s\n--- want ---\n%s", got, sampleSettings)
	}
}

func TestApplyTouchesOnlyOwnedKeys(t *testing.T) {
	doc, err := LoadDocument(t.TempDir() + "/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse([]byte(sampleSettings)); err != nil {
		t.Fatal(err)
	}
	doc.Root = mustParse(t, sampleSettings).(*Object)

	res := doc.Apply(Patch{
		SetEnv: map[string]string{
			"ANTHROPIC_BASE_URL": "https://new.example.com",
			"ANTHROPIC_AUTH_TOKEN": "sk-new",
		},
		ClearEnv: []string{"ANTHROPIC_DEFAULT_FABLE_MODEL"},
		SetModelCompact: map[string]int64{
			"packy-officially/deepseek-flash": 450000,
		},
	})
	if !res.Changed {
		t.Fatal("expected a change")
	}

	out, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)

	for _, want := range []string{
		`"ANTHROPIC_BASE_URL": "https://new.example.com"`,
		`"ANTHROPIC_AUTH_TOKEN": "sk-new"`,
		`"KEEP_ME": "yes"`,
		`"model": "fable"`,
		`"WebSearch"`,
		`"command": "rtk hook claude"`,
		`"autoCompactWindow": 450000`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("output is missing %s\n%s", want, text)
		}
	}
	// Order inside env must survive: base URL stays first.
	if strings.Index(text, "ANTHROPIC_BASE_URL") > strings.Index(text, "KEEP_ME") {
		t.Errorf("env key order changed:\n%s", text)
	}

	// The result must still be valid JSON.
	var check map[string]any
	if err := json.Unmarshal(out, &check); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
}

func TestApplyIsIdempotent(t *testing.T) {
	doc := &Document{Path: "x", Root: mustParse(t, sampleSettings).(*Object), indent: "  ", trailingNewline: true}
	p := Patch{
		SetEnv:          map[string]string{"ANTHROPIC_AUTH_TOKEN": "sk-new"},
		ClearEnv:        []string{"ANTHROPIC_AUTO_COMPACT_WINDOW"},
		SetModelCompact: map[string]int64{"m": 900000},
		TopCompact:      ptr(int64(450000)),
	}
	if res := doc.Apply(p); !res.Changed {
		t.Fatal("first apply must change something")
	}
	if res := doc.Apply(p); res.Changed {
		t.Fatalf("second apply must be a no-op, changed: %s", res.Summary())
	}
}

func TestApplyCreatesAndRemovesEmptyContainers(t *testing.T) {
	doc := &Document{Path: "x", Root: NewObject(), indent: "  ", trailingNewline: true}
	doc.Apply(Patch{SetEnv: map[string]string{"A": "1"}})
	if doc.Root.Object("env") == nil {
		t.Fatal("env should have been created")
	}
	doc.Apply(Patch{ClearEnv: []string{"A"}})
	if doc.Root.Has("env") {
		t.Errorf("an env block ccsc created and emptied must be removed again: %v", doc.Root.Keys())
	}
}

func TestApplyKeepsPreexistingEmptyEnvBlock(t *testing.T) {
	root := NewObject()
	root.Set("env", NewObject())
	doc := &Document{Path: "x", Root: root, indent: "  ", trailingNewline: true}
	doc.Apply(Patch{SetEnv: map[string]string{"A": "1"}})
	doc.Apply(Patch{ClearEnv: []string{"A"}})
	if !doc.Root.Has("env") {
		t.Error("an env block the user wrote must survive becoming empty")
	}
}

func TestDetectIndent(t *testing.T) {
	cases := map[string]string{
		"{\n  \"a\": 1\n}": "  ",
		"{\n    \"a\": 1\n}": "    ",
		"{\n\t\"a\": 1\n}":   "\t",
		"{}":                 "  ",
	}
	for in, want := range cases {
		if got := detectIndent([]byte(in)); got != want {
			t.Errorf("detectIndent(%q) = %q, want %q", in, got, want)
		}
	}
}

func mustParse(t *testing.T, s string) Value {
	t.Helper()
	v, err := Parse([]byte(s))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return v
}

func ptr[T any](v T) *T { return &v }
