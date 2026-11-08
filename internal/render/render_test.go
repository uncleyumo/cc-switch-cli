package render

import (
	"reflect"
	"testing"

	"github.com/uncleyumo/ccsc/internal/group"
)

func sample() *group.Group {
	return &group.Group{
		Schema: 1,
		Name:   "cost-effective",
		BaseURL: "https://new-api.example.com/",
		Token:   "sk-test",
		Window:  500000,
		Slots: map[group.SlotName]group.Slot{
			group.Fable:    {Model: "packy-officially/deepseek-flash", Name: "DeepSeek 官方"},
			group.Opus:     {Model: "packy-cc-sale/claude-opus-5-5", Window: group.OneMillion},
			group.Sonnet:   {Model: "packy-deepseek-sale/deepseek-flash"},
			group.Haiku:    {Model: "opencode2api-zen/glm-5.3-flash-free", Compact: 150000},
			group.Subagent: {Model: "opencode2api-zen/glm-5.3-flash-free"},
		},
	}
}

func TestGlobalRendersEveryOwnedKey(t *testing.T) {
	p := Global(sample(), Options{})

	if got, want := p.SetEnv[EnvBaseURL], "https://new-api.example.com"; got != want {
		t.Errorf("%s = %q, want %q", EnvBaseURL, got, want)
	}
	if got, want := p.SetEnv[EnvToken], "sk-test"; got != want {
		t.Errorf("%s = %q, want %q", EnvToken, got, want)
	}
	if got, want := p.SetEnv[DefaultModelKey(group.Fable)], "packy-officially/deepseek-flash"; got != want {
		t.Errorf("fable model = %q, want %q", got, want)
	}
	if got, want := p.SetEnv[DefaultModelNameKey(group.Fable)], "DeepSeek 官方"; got != want {
		t.Errorf("fable name = %q, want %q", got, want)
	}
	if got, want := p.SetEnv[DefaultModelKey(group.Opus)], "packy-cc-sale/claude-opus-5-5[1m]"; got != want {
		t.Errorf("opus model = %q, want %q (1M marker)", got, want)
	}
	if got, want := p.SetEnv[EnvMaxContextTokens], "500000"; got != want {
		t.Errorf("%s = %q, want %q", EnvMaxContextTokens, got, want)
	}
	if got, want := p.SetEnv[EnvSubagentModel], "opencode2api-zen/glm-5.3-flash-free"; got != want {
		t.Errorf("%s = %q, want %q", EnvSubagentModel, got, want)
	}
	// Default startup slot is sonnet.
	if got, want := p.SetEnv[EnvDefaultModel], "packy-deepseek-sale/deepseek-flash"; got != want {
		t.Errorf("%s = %q, want %q", EnvDefaultModel, got, want)
	}
}

func TestGlobalClearsShadowingVariables(t *testing.T) {
	p := Global(sample(), Options{})
	for _, k := range []string{EnvAutoCompactWindow, EnvAutoCompactWindowTokens} {
		if _, set := p.SetEnv[k]; set {
			t.Errorf("%s must not be written", k)
		}
		if !contains(p.ClearEnv, k) {
			t.Errorf("%s must be cleared, got %v", k, p.ClearEnv)
		}
	}
}

func TestGlobalCompactDefaults(t *testing.T) {
	p := Global(sample(), Options{})

	// 1M slot -> 900k.
	if got, want := p.SetModelCompact["packy-cc-sale/claude-opus-5-5"], int64(900000); got != want {
		t.Errorf("opus compact = %d, want %d", got, want)
	}
	// Plain slot inherits the group window (500k) -> 450k.
	if got, want := p.SetModelCompact["packy-officially/deepseek-flash"], int64(450000); got != want {
		t.Errorf("fable compact = %d, want %d", got, want)
	}
	// Explicit value wins.
	if got, want := p.SetModelCompact["opencode2api-zen/glm-5.3-flash-free"], int64(150000); got != want {
		t.Errorf("haiku compact = %d, want %d", got, want)
	}
	// The 1M marker is stripped from the modelSettings key.
	for id := range p.SetModelCompact {
		if id == "packy-cc-sale/claude-opus-5-5[1m]" {
			t.Fatalf("modelSettings key must not carry the [1m] marker")
		}
	}
}

func TestGlobalNoWindowWritesNothingForCompact(t *testing.T) {
	g := sample()
	g.Window = 0
	p := Global(g, Options{})

	if !contains(p.ClearEnv, EnvMaxContextTokens) {
		t.Errorf("no group window must clear %s", EnvMaxContextTokens)
	}
	// Only the slot with an explicit compact survives.
	want := map[string]int64{"opencode2api-zen/glm-5.3-flash-free": 150000, "packy-cc-sale/claude-opus-5-5": 900000}
	if !reflect.DeepEqual(p.SetModelCompact, want) {
		t.Errorf("compact = %v, want %v", p.SetModelCompact, want)
	}
}

func TestStartupSlotOption(t *testing.T) {
	p := Global(sample(), Options{StartupSlot: group.Fable})
	if got, want := p.SetEnv[EnvDefaultModel], "packy-officially/deepseek-flash"; got != want {
		t.Errorf("%s = %q, want %q", EnvDefaultModel, got, want)
	}
	// Subagent is not a valid startup slot; it falls back to sonnet.
	p = Global(sample(), Options{StartupSlot: group.Subagent})
	if got, want := p.SetEnv[EnvDefaultModel], "packy-deepseek-sale/deepseek-flash"; got != want {
		t.Errorf("%s = %q, want %q", EnvDefaultModel, got, want)
	}
}

func TestProjectUsesSingleTopLevelWindow(t *testing.T) {
	p := Project(sample(), Options{})

	if len(p.SetModelCompact) != 0 {
		t.Errorf("project layer must not write modelSettings, got %v", p.SetModelCompact)
	}
	if p.TopCompact == nil {
		t.Fatal("project layer must write a top-level autoCompactWindow")
	}
	// Values are 900k (opus), 450k (others), 150k (haiku) -> minimum wins.
	if got, want := *p.TopCompact, int64(150000); got != want {
		t.Errorf("top compact = %d, want %d", got, want)
	}
}

func TestClearRemovesEverythingOwned(t *testing.T) {
	p := Clear()
	for _, k := range OwnedEnvKeys {
		if !contains(p.ClearEnv, k) {
			t.Errorf("%s missing from the clear plan", k)
		}
	}
	if !p.ClearTopCompact {
		t.Error("clear must remove the top-level autoCompactWindow")
	}
}

func TestEmptySubagentIsCleared(t *testing.T) {
	g := sample()
	delete(g.Slots, group.Subagent)
	p := Global(g, Options{})
	if _, set := p.SetEnv[EnvSubagentModel]; set {
		t.Error("an empty subagent slot must not write CLAUDE_CODE_SUBAGENT_MODEL")
	}
	if !contains(p.ClearEnv, EnvSubagentModel) {
		t.Error("an empty subagent slot must clear a leftover CLAUDE_CODE_SUBAGENT_MODEL")
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
