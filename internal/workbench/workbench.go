// Package workbench holds the two screens ccsc uses: a list and a panel.
//
// A panel is one screen for a whole configuration. Rows are edited in place,
// model ids are typed with inline suggestions instead of opening a picker, and
// secrets stay masked until asked for. Commands sequence these screens; no
// screen opens another.
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
}

// ListResult is the choice the user made.
type ListResult struct {
	Action Action
	Item   string
}

type listModel struct {
	opt    ListOptions
	cursor int
	result ListResult
}

// RunList shows a list and returns the chosen action.
func RunList(opt ListOptions) (ListResult, error) {
	if len(opt.Bindings) == 0 {
		opt.Bindings = []Binding{
			{Key: "enter", Label: "select", Action: ActEnter},
			{Key: "q", Label: "quit", Action: ActQuit},
		}
	}
	m := &listModel{opt: opt, cursor: opt.Cursor}
	if m.cursor >= len(opt.Items) {
		m.cursor = 0
	}
	final, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		return ListResult{}, err
	}
	return final.(*listModel).result, nil
}

func (m *listModel) Init() tea.Cmd { return nil }

func (m *listModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
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

	if len(m.opt.Items) == 0 {
		empty := m.opt.Empty
		if empty == "" {
			empty = "(empty)"
		}
		b.WriteString("  " + dimStyle.Render(empty) + "\n")
	}
	for i, item := range m.opt.Items {
		if i == m.cursor {
			b.WriteString(selStyle.Render("> "+item) + "\n")
		} else {
			b.WriteString("  " + item + "\n")
		}
	}

	if m.opt.Status != "" {
		b.WriteString("\n" + okStyle.Render(m.opt.Status) + "\n")
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
	// RowSecret is masked until revealed with "v"; typing replaces it.
	RowSecret
	// RowSuggest is a string with an inline list of suggestions.
	RowSuggest
	// RowReadOnly cannot be edited.
	RowReadOnly
)

// Row is one line of a panel.
type Row struct {
	Section string
	Label   string
	Kind    RowKind
	Value   string
	Suggest []string
	Hint    string
	// Empty is the placeholder shown when Value is empty.
	Empty string
}

// PanelOptions configures a panel.
type PanelOptions struct {
	Title  string
	Header []string
	Rows   []Row
	// Footer replaces the default key hints.
	Footer string
	// Validate runs on every edit. A non-nil error is shown and blocks saving.
	Validate func(values []string) error
	// Fetch is bound to ctrl+f. It returns suggestions that are merged into
	// every RowSuggest row, plus a one-line status to show above the footer.
	Fetch func(values []string) ([]string, string, error)
}

// PanelResult is the submitted values, in row order.
type PanelResult struct {
	Saved  bool
	Values []string
}

const (
	secretHold   = 10 * time.Second
	maxSuggest   = 6
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
	dynamic     []string
	status      string
	result      PanelResult
}

// RunPanel shows a panel and returns the submitted values.
func RunPanel(opt PanelOptions) (PanelResult, error) {
	m := &panelModel{opt: opt, height: 24}
	m.values = make([]string, len(opt.Rows))
	for i, r := range opt.Rows {
		m.values[i] = r.Value
	}
	final, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		return PanelResult{}, err
	}
	return final.(*panelModel).result, nil
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

	switch key.String() {
	case "ctrl+c", "esc":
		return m, tea.Quit
	case "ctrl+s":
		m.validate()
		if m.validateErr != "" {
			return m, nil
		}
		m.result = PanelResult{Saved: true, Values: append([]string(nil), m.values...)}
		return m, tea.Quit
	case "up", "k", "shift+tab", "left":
		m.move(-1)
		return m, nil
	case "down", "j", "tab", "right", "enter":
		if row.Kind == RowSuggest && key.String() != "tab" {
			if match := m.bestMatch(m.values[m.focus]); match != "" {
				m.values[m.focus] = match
			}
		}
		m.move(1)
		return m, nil
	case "backspace":
		if row.Kind == RowReadOnly {
			return m, nil
		}
		m.typeInto(row, "", true)
		return m, nil
	case "ctrl+u":
		if row.Kind != RowReadOnly {
			m.values[m.focus] = ""
			m.replacing = false
		}
		return m, nil
	case "ctrl+f":
		if m.opt.Fetch == nil {
			return m, nil
		}
		suggest, status, err := m.opt.Fetch(m.values)
		if err != nil {
			m.status = ""
			m.validateErr = err.Error()
			return m, nil
		}
		m.dynamic = suggest
		m.status = status
		return m, nil
	case "v":
		if row.Kind == RowSecret {
			m.revealed = !m.revealed
			if m.revealed {
				return m, tea.Tick(secretHold, func(time.Time) tea.Msg { return hideSecretMsg{} })
			}
			return m, nil
		}
	}

	if row.Kind == RowReadOnly {
		return m, nil
	}
	if key.Type == tea.KeyRunes && len(key.Runes) > 0 {
		m.typeInto(row, string(key.Runes), false)
	}
	return m, nil
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
	m.validate()
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

func (m *panelModel) validate() {
	if m.opt.Validate == nil {
		m.validateErr = ""
		return
	}
	if err := m.opt.Validate(m.values); err != nil {
		m.validateErr = err.Error()
		return
	}
	m.validateErr = ""
}

// matches returns the suggestions that contain what has been typed so far.
func (m *panelModel) matches(idx int) []string {
	row := m.opt.Rows[idx]
	if row.Kind != RowSuggest {
		return nil
	}
	pool := row.Suggest
	if len(m.dynamic) > 0 {
		pool = m.dynamic
	}
	if len(pool) == 0 {
		return nil
	}
	needle := strings.ToLower(strings.TrimSpace(m.values[idx]))
	var out []string
	for _, s := range pool {
		if needle != "" && !strings.Contains(strings.ToLower(s), needle) {
			continue
		}
		out = append(out, s)
		if len(out) >= maxSuggest {
			break
		}
	}
	return out
}

func (m *panelModel) bestMatch(value string) string {
	ms := m.matches(m.focus)
	if len(ms) == 0 {
		return ""
	}
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return ms[0]
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

// visibleRows counts a row plus the suggestion lines it adds.
func (m *panelModel) rowHeight(i int) int {
	h := 1
	if m.opt.Rows[i].Section != "" {
		h++
	}
	if i == m.focus {
		h += len(m.matches(i))
	}
	return h
}

func (m *panelModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(m.opt.Title) + "\n")
	for _, h := range m.opt.Header {
		b.WriteString("  " + h + "\n")
	}
	b.WriteString("\n")

	// Keep the focused row inside the window.
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
				b.WriteString(hintStyle.Render("  v to reveal"))
			}
		case RowSuggest:
			v := m.values[i]
			if v == "" {
				b.WriteString(dimStyle.Render(row.Empty))
			} else {
				b.WriteString(v)
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
		}
		if row.Hint != "" && i == m.focus {
			b.WriteString(hintStyle.Render("  " + row.Hint))
		}
		b.WriteString("\n")
		lines++

		if i == m.focus {
			for _, s := range m.matches(i) {
				if lines >= m.windowSize() {
					break
				}
				style := hintStyle
				if s == m.values[i] {
					style = okStyle
				}
				b.WriteString("  " + strings.Repeat(" ", labelW) + style.Render(s) + "\n")
				lines++
			}
		}
	}

	if m.status != "" {
		b.WriteString("\n" + okStyle.Render(m.status) + "\n")
	}
	if m.validateErr != "" {
		b.WriteString("\n" + errStyle.Render("error: "+m.validateErr) + "\n")
	}

	footer := m.opt.Footer
	if footer == "" {
		footer = "up/down move   left/right previous/next   enter accept & next   ctrl+s save   esc cancel"
		if m.opt.Fetch != nil {
			footer = "ctrl+f fetch models   " + footer
		}
	}
	b.WriteString("\n" + hintStyle.Render(footer))
	return b.String()
}

// usedLines is how many terminal lines rows [from, to) occupy.
func (m *panelModel) usedLines(from, to int) int {
	n := 0
	for i := from; i < to; i++ {
		n += m.rowHeight(i)
	}
	return n
}

// Confirm asks a yes/no question on its own screen.
func Confirm(title string, yes string, no string) (bool, error) {
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
