package workbench

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "ctrl+s":
		return tea.KeyMsg{Type: tea.KeyCtrlS}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func newPanel(rows []Row) *panelModel {
	m := &panelModel{opt: PanelOptions{Title: "panel", Rows: rows}, height: 40}
	m.values = make([]string, len(rows))
	for i, r := range rows {
		m.values[i] = r.Value
	}
	return m
}

func TestPanelMasksSecrets(t *testing.T) {
	m := newPanel([]Row{
		{Section: "endpoint", Label: "token", Kind: RowSecret, Value: "sk-abcdef123456"},
	})
	view := m.View()
	if strings.Contains(view, "sk-abcdef123456") {
		t.Fatalf("the secret leaked into the frame:\n%s", view)
	}
	if !strings.Contains(view, "********") {
		t.Fatalf("expected a masked value:\n%s", view)
	}

	m.revealed = true
	if !strings.Contains(m.View(), "sk-abcdef123456") {
		t.Fatal("reveal did not show the value")
	}
}

func TestPanelTypingReplacesASecret(t *testing.T) {
	m := newPanel([]Row{{Label: "token", Kind: RowSecret, Value: "old-token"}})
	m.typeInto(m.opt.Rows[0], "n", false)
	if got := m.values[0]; got != "n" {
		t.Fatalf("first keystroke should replace the secret, got %q", got)
	}
	m.typeInto(m.opt.Rows[0], "e", false)
	if got := m.values[0]; got != "ne" {
		t.Fatalf("subsequent keystrokes append, got %q", got)
	}
}

func TestPanelShowsFilteredSuggestions(t *testing.T) {
	m := newPanel([]Row{
		{Label: "fable", Kind: RowSuggest, Value: "glm", Suggest: []string{
			"packy-officially/deepseek-flash",
			"opencode2api-zen/glm-5.3-flash-free",
			"packy-glm-sale/glm-5.3",
		}},
	})
	view := m.View()
	if strings.Contains(view, "deepseek-flash") {
		t.Errorf("a non-matching suggestion was shown:\n%s", view)
	}
	if !strings.Contains(view, "glm-5.3") {
		t.Errorf("matching suggestions missing:\n%s", view)
	}
}

func TestPanelEnterAcceptsTheTopSuggestion(t *testing.T) {
	rows := []Row{
		{Label: "fable", Kind: RowSuggest, Value: "glm", Suggest: []string{"a/glm-5.3", "b/glm-4"}},
		{Label: "opus", Kind: RowText, Empty: "not set"},
	}
	m := newPanel(rows)
	m.handleKey(keyMsg("enter"))
	if got := m.values[0]; got != "a/glm-5.3" {
		t.Fatalf("enter did not accept the top match, got %q", got)
	}
	if m.focus != 1 {
		t.Fatalf("enter should also advance, focus=%d", m.focus)
	}
}

func TestPanelNavigationWraps(t *testing.T) {
	m := newPanel([]Row{{Label: "a", Kind: RowText}, {Label: "b", Kind: RowText}})
	m.handleKey(keyMsg("left"))
	if m.focus != 1 {
		t.Fatalf("left from the first row should wrap to the last, got %d", m.focus)
	}
	m.handleKey(keyMsg("right"))
	if m.focus != 0 {
		t.Fatalf("right from the last row should wrap to the first, got %d", m.focus)
	}
}

func TestPanelValidationBlocksSaving(t *testing.T) {
	m := newPanel([]Row{{Label: "name", Kind: RowText}})
	m.opt.Validate = func(values []string) error {
		if values[0] == "" {
			return errTest("name is required")
		}
		return nil
	}
	m.handleKey(keyMsg("ctrl+s"))
	if m.result.Saved {
		t.Fatal("an invalid panel must not save")
	}
	if !strings.Contains(m.View(), "name is required") {
		t.Errorf("the error should be visible:\n%s", m.View())
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }
