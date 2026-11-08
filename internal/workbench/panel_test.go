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
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "ctrl+s":
		return tea.KeyMsg{Type: tea.KeyCtrlS}
	case "ctrl+r":
		return tea.KeyMsg{Type: tea.KeyCtrlR}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func newPanel(rows []Row) *panelModel {
	m := &panelModel{opt: PanelOptions{Title: "panel", Rows: rows}, height: 40, result: PanelResult{PickRow: -1}}
	m.values = make([]string, len(rows))
	for i, r := range rows {
		m.values[i] = r.Value
	}
	return m
}

func TestPanelMasksSecrets(t *testing.T) {
	m := newPanel([]Row{{Section: "endpoint", Label: "token", Kind: RowSecret, Value: "sk-abcdef123456"}})
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

// TestPanelLettersAreTypedNotBound is the regression test for the bug that
// made model ids impossible to type: j and k used to move the cursor instead
// of inserting characters.
func TestPanelLettersAreTypedNotBound(t *testing.T) {
	m := newPanel([]Row{
		{Label: "base url", Kind: RowText},
		{Label: "other", Kind: RowText},
	})
	for _, ch := range []string{"p", "a", "c", "k", "y", "-", "j", "k", "v", "q"} {
		m.handleKey(keyMsg(ch))
	}
	if got, want := m.values[0], "packy-jkvq"; got != want {
		t.Fatalf("typed value = %q, want %q", got, want)
	}
	if m.focus != 0 {
		t.Fatalf("focus moved while typing: %d", m.focus)
	}
}

func TestPanelEnterOnAPickRowRequestsThePicker(t *testing.T) {
	m := newPanel([]Row{
		{Label: "fable", Kind: RowPick, Empty: "not set"},
		{Label: "opus", Kind: RowText},
	})
	m.handleKey(keyMsg("enter"))
	if m.result.PickRow != 0 {
		t.Fatalf("PickRow = %d, want 0", m.result.PickRow)
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

func TestPickerFiltersAsYouType(t *testing.T) {
	m := &listModel{opt: ListOptions{
		Title:  "fable model",
		Items:  []string{"packy-officially/deepseek-flash", "opencode2api-zen/glm-5.3-flash-free", "origin-mimo/mimo-v2.6-flash"},
		Filter: true,
	}}
	for _, ch := range []string{"j", "k"} {
		m.Update(keyMsg(ch))
	}
	if m.query != "jk" {
		t.Fatalf("query = %q, want typing to be captured verbatim", m.query)
	}
	if got := m.visible(); len(got) != 0 {
		t.Fatalf("no item contains jk, got %v", got)
	}

	m.query = ""
	for _, ch := range []string{"g", "l", "m"} {
		m.Update(keyMsg(ch))
	}
	items := m.visible()
	if len(items) != 1 || !strings.Contains(items[0], "glm") {
		t.Fatalf("glm should match one model, got %v", items)
	}

	m.Update(keyMsg("enter"))
	if !strings.Contains(m.result.Item, "glm") {
		t.Fatalf("enter should select the highlighted match, got %q", m.result.Item)
	}
}

func TestInitialFocusStaysWhereTheUserWas(t *testing.T) {
	rows := []Row{
		{Label: "base url", Kind: RowText},
		{Label: "fable", Kind: RowPick},
		{Label: "opus", Kind: RowPick},
	}
	if got := initialFocus(rows, 2); got != 2 {
		t.Fatalf("focus = %d, want 2 (the row the picker was opened from)", got)
	}
	if got := initialFocus(rows, 0); got != 0 {
		t.Fatalf("focus = %d, want 0", got)
	}
	rows[2].Kind = RowReadOnly
	if got := initialFocus(rows, 2); got != 0 {
		t.Fatalf("a read-only request must fall back to the first editable row, got %d", got)
	}
}

func TestPanelIgnoresControlCharacters(t *testing.T) {
	m := newPanel([]Row{{Label: "auto-compact", Kind: RowText, Value: "450k"}})
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{0x00}})
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{0x1b}})
	if got := m.values[0]; got != "450k" {
		t.Fatalf("a control character was appended: %q", got)
	}
	m.handleKey(keyMsg("0"))
	if got := m.values[0]; got != "450k0" {
		t.Fatalf("a normal character should still append: %q", got)
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }
