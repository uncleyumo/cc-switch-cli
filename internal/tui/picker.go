package tui

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// pickerItem is one row in the picker list.
type pickerItem struct {
	name   string
	active bool
}

func (p pickerItem) Title() string {
	if p.active {
		return "● " + p.name
	}
	return "  " + p.name
}
func (p pickerItem) Description() string { return "" }
func (p pickerItem) FilterValue() string { return p.name }

// pickerDelegate renders each item compactly on a single line.
type pickerDelegate struct{}

func (pickerDelegate) Height() int                             { return 1 }
func (pickerDelegate) Spacing() int                            { return 0 }
func (pickerDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (pickerDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	it, ok := item.(pickerItem)
	if !ok {
		return
	}
	line := it.Title()
	if index == m.Index() {
		line = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true).Render("› " + line[2:])
	} else if it.active {
		line = activeStyle.Render(line)
	}
	fmt.Fprint(w, line)
}

// pickerModel is the Bubble Tea model behind RunPicker.
type pickerModel struct {
	list     list.Model
	chosen   string
	quitting bool
}

func newPickerModel(names []string, current, title string) pickerModel {
	items := make([]list.Item, 0, len(names))
	for _, n := range names {
		items = append(items, pickerItem{name: n, active: n == current})
	}
	l := list.New(items, pickerDelegate{}, 40, min(len(items)+4, 20))
	l.Title = title
	l.Styles.Title = titleStyle
	l.SetShowStatusBar(false)
	l.SetShowHelp(true)
	l.SetFilteringEnabled(true)
	return pickerModel{list: l}
}

func (m pickerModel) Init() tea.Cmd { return nil }

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height)
	case tea.KeyMsg:
		if m.list.FilterState() == list.Filtering {
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			return m, cmd
		}
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.quitting = true
			return m, tea.Quit
		case "enter":
			if it, ok := m.list.SelectedItem().(pickerItem); ok {
				m.chosen = it.name
				return m, tea.Quit
			}
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m pickerModel) View() string {
	if m.quitting && m.chosen == "" {
		return helpStyle.Render("cancelled\n")
	}
	return m.list.View()
}

// RunPicker drives a picker Bubble Tea program and returns the chosen name
// ("" if the user cancelled).
func RunPicker(names []string, current, title string) (string, error) {
	m := newPickerModel(names, current, title)
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return "", err
	}
	if pm, ok := final.(pickerModel); ok {
		return pm.chosen, nil
	}
	return "", nil
}
