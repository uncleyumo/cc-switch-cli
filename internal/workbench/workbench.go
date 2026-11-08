// Package workbench holds the two screens ccsc uses: a list and a panel.
//
// A panel is one screen for a whole configuration. Text is typed in place;
// rows that pick from a list hand off to a picker and come back. No plain
// letter is ever bound to a command inside a panel or a filterable list, so
// typing can never be swallowed by a shortcut.
package workbench

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/uncleyumo/ccsc/internal/ui"
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true)
	dimStyle   = lipgloss.NewStyle().Faint(true)
	selStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
	okStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("78"))
	errStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	hintStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
)

// Action is what a list screen decided.
type Action int

const (
	ActNone Action = iota
	ActEnter
	ActNew
	ActEdit
	ActDelete
	ActApply
	ActQuit
)

// Binding maps a key to an action and its footer label.
type Binding struct {
	Key    string
	Label  string
	Action Action
}

// ListOptions configures a list screen.
type ListOptions struct {
	Title    string
	Items    []string
	Header   []string
	Bindings []Binding
	Status   string
	Empty    string
	// Cursor is the initially selected row.
	Cursor int
	// Filter turns the screen into a picker: typed characters filter the
	// items and no letter key is bound to anything.
	Filter bool
}

// ListResult is the choice the user made.
type ListResult struct {
	Action Action
	Item   string
}

type listModel struct {
	opt     ListOptions
	cursor  int
	query   string
	cancel  bool
	result  ListResult
}

// RunList shows a list and returns the chosen action.
func RunList(opt ListOptions) (ListResult, error) {
	if !opt.Filter && len(opt.Bindings) == 0 {
		opt.Bindings = []Binding{
			{Key: "enter", Label: "select", Action: ActEnter},
			{Key: "q", Label: "quit", Action: ActQuit},
		}
	}
	m := &listModel{opt: opt, cursor: opt.Cursor}
	if m.cursor >= len(opt.Items) || m.cursor < 0 {
		m.cursor = 0
	}
	final, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		return ListResult{}, err
	}
	return final.(*listModel).result, nil
}

// RunPicker is the filterable list used to choose a model or a slot name.
func RunPicker(title string, items []string, current string) (string, bool, error) {
	cursor := 0
	for i, item := range items {
		if item == current {
			cursor = i
			break
		}
	}
	res, err := RunList(ListOptions{Title: title, Items: items, Filter: true, Cursor: cursor})
	if err != nil {
		return "", false, err
	}
	if res.Item == "" {
		return "", false, nil
	}
	return res.Item, true, nil
}

func (m *listModel) Init() tea.Cmd { return nil }

// visible returns the items matching the current query.
func (m *listModel) visible() []string {
	if m.query == "" {
		return m.opt.Items
	}
	needle := strings.ToLower(m.query)
	var out []string
	for _, item := range m.opt.Items {
		if strings.Contains(strings.ToLower(item), needle) {
			out = append(out, item)
		}
	}
	return out
}

func (m *listModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	if m.opt.Filter {
		switch key.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit
		case "enter":
			if items := m.visible(); len(items) > 0 {
				m.result = ListResult{Action: ActEnter, Item: items[m.cursor]}
			}
			return m, tea.Quit
		case "up":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case "down":
			if m.cursor < len(m.visible())-1 {
				m.cursor++
			}
			return m, nil
		case "backspace":
			if r := []rune(m.query); len(r) > 0 {
				m.query = string(r[:len(r)-1])
				m.cursor = 0
			}
			return m, nil
		case "ctrl+u":
			m.query = ""
			m.cursor = 0
			return m, nil
		}
		if key.Type == tea.KeyRunes {
			if text := printable(key.Runes); text != "" {
				m.query += text
				m.cursor = 0
			}
		}
		return m, nil
	}

	switch key.String() {
	case "ctrl+c", "q", "esc":
		m.result = ListResult{Action: ActQuit}
		return m, tea.Quit
	case "up", "k", "left":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j", "right":
		if m.cursor < len(m.opt.Items)-1 {
			m.cursor++
		}
	}
	current := ""
	if len(m.opt.Items) > 0 {
		current = m.opt.Items[m.cursor]
	}
	for _, b := range m.opt.Bindings {
		if key.String() != b.Key {
			continue
		}
		switch b.Action {
		case ActNew:
			m.result = ListResult{Action: ActNew}
			return m, tea.Quit
		case ActQuit:
			m.result = ListResult{Action: ActQuit}
			return m, tea.Quit
		default:
			if current == "" {
				continue
			}
			m.result = ListResult{Action: b.Action, Item: current}
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *listModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(m.opt.Title) + "\n")
	for _, h := range m.opt.Header {
		b.WriteString("  " + h + "\n")
	}
	b.WriteString("\n")

	items := m.visible()
	if m.opt.Filter {
		line := "  filter: " + m.query
		if m.query == "" {
			line = "  filter: " + hintStyle.Render("type to narrow the list")
		}
		b.WriteString(line + "\n\n")
	}

	if len(items) == 0 {
		empty := m.opt.Empty
		if empty == "" {
			empty = "(nothing matches)"
		}
		b.WriteString("  " + dimStyle.Render(empty) + "\n")
	}
	for i, item := range items {
		if i == m.cursor {
			b.WriteString(selStyle.Render("> "+item) + "\n")
		} else {
			b.WriteString("  " + item + "\n")
		}
	}

	if m.opt.Status != "" {
		b.WriteString("\n" + okStyle.Render(m.opt.Status) + "\n")
	}

	if m.opt.Filter {
		b.WriteString("\n" + hintStyle.Render("type to filter   up/down move   enter select   esc cancel"))
		return b.String()
	}
	var keys []string
	for _, bind := range m.opt.Bindings {
		keys = append(keys, bind.Key+" "+bind.Label)
	}
	b.WriteString("\n" + hintStyle.Render(strings.Join(keys, "   ")))
	return b.String()
}

// RowKind is how a panel row is displayed and edited.
type RowKind int

const (
	// RowText is a plain editable string.
	RowText RowKind = iota
	// RowSecret is masked until revealed; typing replaces it.
	RowSecret
	// RowPick shows a value chosen elsewhere; enter opens a picker.
	RowPick
	// RowReadOnly cannot be edited.
	RowReadOnly
)

// Row is one line of a panel.
type Row struct {
	Section string
	Label   string
	Kind    RowKind
	Value   string
	Hint    string
	// Empty is the placeholder shown when Value is empty.
	Empty string
}

// PanelOptions configures a panel.
type PanelOptions struct {
	Title  string
	Header []string
	Rows   []Row
	// Focus is the row to start on. Zero is the first row; a read-only row
	// falls back to the first editable one. Callers set it when reopening a
	// panel after a picker so the cursor stays where the user left it.
	Focus int
	// Footer replaces the default key hints.
	Footer   string
	Status   string
	Error    string
	Validate func(values []string) error
}

// PanelResult is the submitted values, or a request to open a picker.
type PanelResult struct {
	Saved  bool
	Values []string
	// PickRow is the index of the row whose picker the caller should open,
	// or -1 when the panel finished normally.
	PickRow int
}

const (
	secretHold   = 10 * time.Second
	secretGlyphs = "********"
)

type hideSecretMsg struct{}

type panelModel struct {
	opt         PanelOptions
	values      []string
	focus       int
	offset      int
	height      int
	revealed    bool
	replacing   bool
	validateErr string
	done        bool
	result      PanelResult
}

// RunPanel shows a panel and returns the submitted values or a picker request.
//
// The caller keeps the values between runs: when the panel hands off to a
// picker it returns PickRow, and the next run is seeded with the values the
// user had already typed.
func RunPanel(opt PanelOptions) (PanelResult, error) {
	m := &panelModel{opt: opt, height: 24, result: PanelResult{PickRow: -1}}
	m.values = make([]string, len(opt.Rows))
	for i, r := range opt.Rows {
		m.values[i] = r.Value
	}
	m.focus = initialFocus(opt.Rows, opt.Focus)
	final, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		return PanelResult{}, err
	}
	res := final.(*panelModel).result
	if res.PickRow < 0 {
		res.Values = append([]string(nil), final.(*panelModel).values...)
	}
	return res, nil
}

// initialFocus honours the caller's request when that row can be edited, and
// otherwise falls back to the first editable row.
func initialFocus(rows []Row, want int) int {
	if want >= 0 && want < len(rows) && rows[want].Kind != RowReadOnly {
		return want
	}
	for i, r := range rows {
		if r.Kind != RowReadOnly {
			return i
		}
	}
	return 0
}

func (m *panelModel) Init() tea.Cmd { return nil }

func (m *panelModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		return m, nil
	case hideSecretMsg:
		m.revealed = false
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *panelModel) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	row := m.opt.Rows[m.focus]

	// Only control keys and arrows are ever commands here. Every printable
	// character belongs to the field being edited.
	switch key.String() {
	case "ctrl+c", "esc":
		return m, tea.Quit
	case "ctrl+s":
		m.collect()
		if m.opt.Validate != nil {
			if err := m.opt.Validate(m.values); err != nil {
				m.validateErr = err.Error()
				return m, nil
			}
		}
		m.result = PanelResult{Saved: true, Values: append([]string(nil), m.values...), PickRow: -1}
		return m, tea.Quit
	case "up", "shift+tab", "left":
		m.move(-1)
		return m, nil
	case "down", "tab", "right":
		m.move(1)
		return m, nil
	case "enter":
		if row.Kind == RowPick {
			m.collect()
			m.result = PanelResult{PickRow: m.focus, Values: append([]string(nil), m.values...)}
			return m, tea.Quit
		}
		m.move(1)
		return m, nil
	case "backspace":
		if row.Kind == RowReadOnly || row.Kind == RowPick {
			return m, nil
		}
		m.typeInto(row, "", true)
		return m, nil
	case "ctrl+u":
		if row.Kind == RowText || row.Kind == RowSecret {
			m.values[m.focus] = ""
			m.replacing = false
			m.validateErr = ""
		}
		return m, nil
	case "ctrl+r":
		if row.Kind == RowSecret {
			m.revealed = !m.revealed
			if m.revealed {
				return m, tea.Tick(secretHold, func(time.Time) tea.Msg { return hideSecretMsg{} })
			}
			return m, nil
		}
	}

	if row.Kind == RowReadOnly || row.Kind == RowPick {
		return m, nil
	}
	if key.Type == tea.KeyRunes {
		// Control characters never belong in a value. Windows terminals
		// deliver Ctrl+Space (the IME toggle) as a NUL rune, which used to be
		// appended silently and then failed validation with an invisible byte.
		if text := printable(key.Runes); text != "" {
			m.typeInto(row, text, false)
		}
	}
	return m, nil
}

// printable drops control characters from typed input.
func printable(rs []rune) string {
	var b strings.Builder
	for _, r := range rs {
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// typeInto appends text, or removes the last rune when backspace is set. A
// secret row is wiped on the first keystroke: replacing a token is the only
// edit anyone does to one.
func (m *panelModel) typeInto(row Row, text string, backspace bool) {
	cur := m.values[m.focus]
	if row.Kind == RowSecret && !m.replacing {
		if backspace {
			return
		}
		cur = ""
		m.replacing = true
	}
	if backspace {
		r := []rune(cur)
		if len(r) == 0 {
			return
		}
		cur = string(r[:len(r)-1])
	} else {
		cur += text
	}
	m.values[m.focus] = cur
	m.validateErr = ""
}

func (m *panelModel) move(delta int) {
	m.focus += delta
	if m.focus < 0 {
		m.focus = len(m.opt.Rows) - 1
	}
	if m.focus >= len(m.opt.Rows) {
		m.focus = 0
	}
	m.revealed = false
	m.replacing = false
}

func (m *panelModel) collect() {
	// Values are kept in sync as they are typed; nothing to gather.
}

func (m *panelModel) windowSize() int {
	if m.height <= 0 {
		return 20
	}
	n := m.height - 10
	if n < 5 {
		n = 5
	}
	return n
}

func (m *panelModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(m.opt.Title) + "\n")
	for _, h := range m.opt.Header {
		b.WriteString("  " + h + "\n")
	}
	b.WriteString("\n")

	if m.focus < m.offset {
		m.offset = m.focus
	}
	for m.offset < m.focus && m.usedLines(m.offset, m.focus) > m.windowSize() {
		m.offset++
	}

	labelW := 0
	for _, r := range m.opt.Rows {
		if w := ui.Width(r.Label); w > labelW {
			labelW = w
		}
	}
	labelW += 2

	lastSection := ""
	lines := 0
	for i := m.offset; i < len(m.opt.Rows) && lines < m.windowSize(); i++ {
		row := m.opt.Rows[i]
		if row.Section != "" && row.Section != lastSection {
			if lines > 0 {
				b.WriteString("\n")
				lines++
			}
			b.WriteString(dimStyle.Render(row.Section) + "\n")
			lines++
			lastSection = row.Section
		}

		marker := "  "
		if i == m.focus {
			marker = selStyle.Render("> ")
		}
		b.WriteString(marker + ui.Pad(row.Label, labelW))

		switch row.Kind {
		case RowSecret:
			if m.revealed {
				b.WriteString(m.values[i])
			} else if m.values[i] == "" {
				b.WriteString(dimStyle.Render(row.Empty))
			} else {
				b.WriteString(secretGlyphs)
			}
			if i == m.focus {
				b.WriteString(hintStyle.Render("  ctrl+r to reveal"))
			}
		case RowPick:
			if m.values[i] == "" {
				b.WriteString(dimStyle.Render(row.Empty))
			} else {
				b.WriteString(m.values[i])
			}
			if i == m.focus {
				b.WriteString(hintStyle.Render("  enter to choose"))
			}
		default:
			v := m.values[i]
			if v == "" {
				b.WriteString(dimStyle.Render(row.Empty))
			} else if row.Kind == RowReadOnly {
				b.WriteString(dimStyle.Render(v))
			} else {
				b.WriteString(v)
			}
			if row.Hint != "" && i == m.focus {
				b.WriteString(hintStyle.Render("  " + row.Hint))
			}
		}
		b.WriteString("\n")
		lines++
	}

	if m.opt.Status != "" {
		b.WriteString("\n" + okStyle.Render(m.opt.Status) + "\n")
	}
	if m.opt.Error != "" {
		b.WriteString("\n" + errStyle.Render("error: "+m.opt.Error) + "\n")
	}
	if m.validateErr != "" {
		b.WriteString("\n" + errStyle.Render("error: "+m.validateErr) + "\n")
	}

	footer := m.opt.Footer
	if footer == "" {
		footer = "up/down move   enter choose or next   ctrl+s save   esc cancel"
	}
	b.WriteString("\n" + hintStyle.Render(footer))
	return b.String()
}

// usedLines is how many terminal lines rows [from, to) occupy.
func (m *panelModel) usedLines(from, to int) int {
	n := 0
	for i := from; i < to; i++ {
		n++
		if m.opt.Rows[i].Section != "" {
			n++
		}
	}
	return n
}

// Confirm asks a yes/no question on its own screen.
func Confirm(title, no, yes string) (bool, error) {
	res, err := RunList(ListOptions{
		Title: title,
		Items: []string{no, yes},
		Bindings: []Binding{
			{Key: "enter", Label: "select", Action: ActEnter},
			{Key: "q", Label: "cancel", Action: ActQuit},
		},
	})
	if err != nil {
		return false, err
	}
	return res.Item == yes, nil
}

// ErrorLine is the shape every command prints on failure.
func ErrorLine(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("error: %v", err)
}
