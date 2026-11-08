package group

import "testing"

func valid() *Group {
	return &Group{
		Schema:  1,
		Name:    "test",
		BaseURL: "https://new-api.example.com",
		Token:   "sk-test",
		Window:  500000,
		Slots: map[SlotName]Slot{
			Fable:  {Model: "a"},
			Opus:   {Model: "b"},
			Sonnet: {Model: "c"},
			Haiku:  {Model: "d"},
		},
	}
}

func TestValidateAcceptsACompleteGroup(t *testing.T) {
	if err := valid().Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRejectsMissingSlotsAndBadInput(t *testing.T) {
	cases := map[string]func(*Group){
		"missing sonnet":     func(g *Group) { delete(g.Slots, Sonnet) },
		"empty base url":     func(g *Group) { g.BaseURL = "" },
		"bad scheme":         func(g *Group) { g.BaseURL = "ftp://x" },
		"empty token":        func(g *Group) { g.Token = "  " },
		"negative window":    func(g *Group) { g.Window = -1 },
		"unknown slot":       func(g *Group) { g.Slots["vision"] = Slot{Model: "x"} },
		"non-1M slot window": func(g *Group) { g.Slots[Opus] = Slot{Model: "b", Window: 500000} },
		"future schema":      func(g *Group) { g.Schema = 99 },
	}
	for name, mutate := range cases {
		g := valid()
		mutate(g)
		if err := g.Validate(); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestRenderedModelNormalisesTheMarker(t *testing.T) {
	cases := []struct {
		slot Slot
		want string
	}{
		{Slot{Model: "m"}, "m"},
		{Slot{Model: "m", Window: OneMillion}, "m[1m]"},
		{Slot{Model: "m[1m]"}, "m[1m]"},
		{Slot{Model: "m[1m]", Window: OneMillion}, "m[1m]"},
		{Slot{Model: "m[1M]"}, "m[1m]"},
	}
	for _, c := range cases {
		if got := c.slot.RenderedModel(); got != c.want {
			t.Errorf("RenderedModel(%+v) = %q, want %q", c.slot, got, c.want)
		}
	}
}

func TestWireModelStripsTheMarker(t *testing.T) {
	if got := (Slot{Model: "m[1m]"}).WireModel(); got != "m" {
		t.Errorf("WireModel = %q, want %q", got, "m")
	}
}

func TestCompactDefaults(t *testing.T) {
	g := valid()
	g.Slots[Opus] = Slot{Model: "b", Window: OneMillion}

	if got, want := g.CompactFor(Sonnet), int64(450000); got != want {
		t.Errorf("sonnet compact = %d, want %d", got, want)
	}
	if got, want := g.CompactFor(Opus), int64(900000); got != want {
		t.Errorf("opus compact = %d, want %d", got, want)
	}
	g.Slots[Sonnet] = Slot{Model: "c", Compact: 123000}
	if got, want := g.CompactFor(Sonnet), int64(123000); got != want {
		t.Errorf("explicit compact = %d, want %d", got, want)
	}

	empty := valid()
	empty.Window = 0
	if got := empty.CompactFor(Sonnet); got != 0 {
		t.Errorf("no window and no explicit value must yield 0, got %d", got)
	}
}

func TestValidateName(t *testing.T) {
	good := []string{"a", "cost-effective", "分组", "a.b", stringsRepeat("x", 64)}
	for _, n := range good {
		if err := ValidateName(n); err != nil {
			t.Errorf("%q should be valid: %v", n, err)
		}
	}
	bad := []string{"", ".hidden", "a/b", `a\b`, "a:b", "CON", "com1", "a..b", stringsRepeat("x", 65)}
	for _, n := range bad {
		if err := ValidateName(n); err == nil {
			t.Errorf("%q should be rejected", n)
		}
	}
}

func TestMaskedToken(t *testing.T) {
	got := MaskedToken("sk-test-1234567890abcdefghijklmnop")
	if got != "sk-tes******mnop" {
		t.Errorf("MaskedToken = %q", got)
	}
	if MaskedToken("") != "(not set)" {
		t.Error("empty token must render as (not set)")
	}
	if got := MaskedToken("short"); got != "*****" {
		t.Errorf("a short token must be fully masked, got %q", got)
	}
}

func stringsRepeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
