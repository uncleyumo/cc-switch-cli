package tui

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// exportItem is one checkable row in the export selector.
type exportItem struct {
	kind    string // "model" or "mcp"
	name    string
	checked bool
	header  bool // non-selectable section header
	label   string
}

func (e exportItem) Title() string {
	if e.header {
		return e.label
	}
	mark := "[ ]"
	if e.checked {
		mark = "[x]"
	}
	return mark + " " + e.name
}
func (e exportItem) Description() string { return "" }
func (e exportItem) FilterValue() string {
	if e.header {
		return ""
	}
	return e.name
}

type exportDelegate struct{}

func (exportDelegate) Height() int                             { return 1 }
func (exportDelegate) Spacing() int                            { return 0 }
func (exportDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (exportDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	it, ok := item.(exportItem)
	if !ok {
		return
	}
	if it.header {
		fmt.Fprint(w, titleStyle.Render(it.label))
		return
	}
	line := it.Title()
	if index == m.Index() {
		line = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true).Render("› " + line)
	}
	fmt.Fprint(w, line)
}

type exportModel struct {
	list     list.Model
	quitting bool
	ok       bool
}

func newExportModel(modelNames, mcpNames []string) exportModel {
	items := make([]list.Item, 0, len(modelNames)+len(mcpNames)+2)
	items = append(items, exportItem{header: true, label: "── Models ──"})
	for _, n := range modelNames {
		items = append(items, exportItem{kind: "model", name: n, checked: true})
	}
	items = append(items, exportItem{header: true, label: "── MCP servers (user scope) ──"})
	for _, n := range mcpNames {
		items = append(items, exportItem{kind: "mcp", name: n, checked: true})
	}

	l := list.New(items, exportDelegate{}, 60, min(len(items)+6, 24))
	l.Title = "cc-switch-cli — export selection"
	l.Styles.Title = titleStyle
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)
	return exportModel{list: l}
}

func (m exportModel) Init() tea.Cmd { return nil }

func (m exportModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height-2)
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			m.quitting = true
			m.ok = false
			return m, tea.Quit
		case "enter":
			m.quitting = true
			m.ok = true
			return m, tea.Quit
		case " ":
			if it, ok := m.list.SelectedItem().(exportItem); ok && !it.header {
				it.checked = !it.checked
				items := m.list.Items()
				items[m.list.Index()] = it
				m.list.SetItems(items)
			}
			return m, nil
		case "x":
			m.setAll(true)
			return m, nil
		case "0":
			m.setAll(false)
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *exportModel) setAll(checked bool) {
	items := m.list.Items()
	for i, raw := range items {
		it, ok := raw.(exportItem)
		if !ok || it.header {
			continue
		}
		it.checked = checked
		items[i] = it
	}
	m.list.SetItems(items)
}

func (m exportModel) View() string {
	hints := helpStyle.Render("space: toggle  •  x: all  •  0: none  •  enter: export  •  esc: cancel")
	return m.list.View() + "\n" + hints
}

func (m exportModel) selections() (models, mcps []string) {
	for _, raw := range m.list.Items() {
		it, ok := raw.(exportItem)
		if !ok || it.header || !it.checked {
			continue
		}
		switch it.kind {
		case "model":
			models = append(models, it.name)
		case "mcp":
			mcps = append(mcps, it.name)
		}
	}
	return models, mcps
}

// RunExportSelector shows two checkable sections (Models / MCP servers).
// ok=false means the user cancelled with esc/q.
func RunExportSelector(modelNames, mcpNames []string) (models, mcps []string, ok bool, err error) {
	m := newExportModel(modelNames, mcpNames)
	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return nil, nil, false, err
	}
	fm, okk := final.(exportModel)
	if !okk {
		return nil, nil, false, nil
	}
	if !fm.ok {
		return nil, nil, false, nil
	}
	models, mcps = fm.selections()
	return models, mcps, true, nil
}
