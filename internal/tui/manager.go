package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cc-switch-cli/internal/manager"
	"cc-switch-cli/internal/model"
	"cc-switch-cli/internal/tester"
	"cc-switch-cli/internal/ui"
)

type managerMode int

const (
	modeList managerMode = iota
	modeView
	modeConfirmDel
	modeTesting
	modeAdd
	modeRename
)

// managerItem is the row shown in the model manager list.
type managerItem struct {
	name     string
	active   bool
	status   *tester.ProviderStatus
	tokenDim string
}

func (m managerItem) Title() string {
	prefix := "  "
	name := m.name
	if m.active {
		prefix = activeStyle.Render("● ")
		name = activeStyle.Render(name)
	}
	suffix := ""
	if m.status != nil {
		var s string
		switch m.status.Status {
		case tester.StatusOK:
			s = statusOK("OK")
		case tester.StatusFail:
			s = statusFail("FAIL")
		case tester.StatusUnsupported:
			s = statusWarn("OK~")
		}
		suffix = "  " + s
	}
	return prefix + name + suffix
}
func (m managerItem) Description() string { return m.tokenDim }
func (m managerItem) FilterValue() string { return m.name }

type managerDelegate struct{}

func (managerDelegate) Height() int                             { return 2 }
func (managerDelegate) Spacing() int                            { return 0 }
func (managerDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (managerDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	it, ok := item.(managerItem)
	if !ok {
		return
	}
	first := it.Title()
	second := helpStyle.Render(it.tokenDim)
	if index == m.Index() {
		first = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true).Render("› " + strings.TrimLeft(first, " ●"))
	}
	fmt.Fprintf(w, "%s\n%s", first, second)
}

// managerModel holds the whole TUI state.
type managerModel struct {
	list        list.Model
	viewport    viewport.Model
	spinner     spinner.Model
	input       textinput.Model
	inputPrompt string // "New model name" or "Rename to"
	renameFrom  string // populated in rename mode
	mode        managerMode
	statuses    map[string]tester.ProviderStatus
	current     string
	statusMsg   string
	err         error
	testTotal   int
	testDone    int
	editHint    string // pre-populated by RunManager after returning from editor
	editRequest string // set when the user pressed `e` and we need to launch the editor
}

// operationDoneMsg fires from tea.Cmd wrappers around blocking manager calls.
type operationDoneMsg struct {
	msg string
	err error
}

type modelsReloadedMsg struct {
	items   []list.Item
	current string
	err     error
}

type testProgressMsg struct{ done, total int }
type testResultsMsg struct {
	results []tester.Result
	err     error
}

// RunManager launches the interactive model manager. When the user presses
// `e`, the manager quits with an editRequest for that model; RunManager then
// opens the editor and re-enters the manager, so the flow feels seamless.
func RunManager() error {
	editRequest := ""
	for {
		m, err := newManagerModel()
		if err != nil {
			return err
		}
		m.editHint = editRequest
		editRequest = ""
		p := tea.NewProgram(m, tea.WithAltScreen())
		final, err := p.Run()
		if err != nil {
			return err
		}
		fm, ok := final.(managerModel)
		if !ok {
			return nil
		}
		if fm.editRequest != "" {
			if err := RunEditor(fm.editRequest); err != nil {
				return fmt.Errorf("edit %s: %w", fm.editRequest, err)
			}
			editRequest = fm.editRequest
			continue
		}
		return nil
	}
}

func newManagerModel() (managerModel, error) {
	names, err := manager.ListModels()
	if err != nil {
		return managerModel{}, err
	}
	current, _ := manager.GetCurrentProvider()
	status, _ := tester.LoadStatus()
	items := buildManagerItems(names, current, status)

	l := list.New(items, managerDelegate{}, 60, 20)
	l.Title = "cc-switch-cli — model manager"
	l.Styles.Title = titleStyle
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	vp := viewport.New(80, 20)

	ti := textinput.New()
	ti.CharLimit = 64
	ti.Width = 48

	sm := managerModel{
		list:     l,
		viewport: vp,
		spinner:  sp,
		input:    ti,
		mode:     modeList,
		current:  current,
	}
	if status != nil {
		sm.statuses = status.Results
	} else {
		sm.statuses = map[string]tester.ProviderStatus{}
	}
	return sm, nil
}

func buildManagerItems(names []string, current string, status *tester.StatusFile) []list.Item {
	items := make([]list.Item, 0, len(names))
	for _, n := range names {
		item := managerItem{name: n, active: n == current}
		if status != nil {
			if s, ok := status.Results[n]; ok {
				sc := s
				item.status = &sc
				if s.TestedAt != "" {
					if t, err := time.Parse(time.RFC3339, s.TestedAt); err == nil {
						item.tokenDim = "last test: " + t.Local().Format("2006-01-02 15:04")
					} else {
						item.tokenDim = "last test: " + s.TestedAt
					}
				}
			}
		}
		items = append(items, item)
	}
	return items
}

func (m managerModel) Init() tea.Cmd { return m.spinner.Tick }

func (m managerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height-2)
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 4
	case operationDoneMsg:
		m.statusMsg = msg.msg
		m.err = msg.err
		return m, m.reloadCmd()
	case modelsReloadedMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.current = msg.current
		m.list.SetItems(msg.items)
	case testProgressMsg:
		m.testDone = msg.done
		m.testTotal = msg.total
	case testResultsMsg:
		m.mode = modeList
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		// Merge into cache and refresh.
		st, _ := tester.LoadStatus()
		if st == nil {
			st = &tester.StatusFile{Results: map[string]tester.ProviderStatus{}}
		}
		st.Upsert(msg.results)
		_ = tester.SaveStatus(st)
		return m, m.reloadCmd()
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case tea.KeyMsg:
		if m.list.FilterState() == list.Filtering {
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			return m, cmd
		}
		switch m.mode {
		case modeView:
			switch msg.String() {
			case "esc", "q", "v", "space":
				m.mode = modeList
				return m, nil
			}
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		case modeConfirmDel:
			switch msg.String() {
			case "y", "Y":
				name := selectedName(m)
				m.mode = modeList
				return m, runOp("removed "+name, func() error { return manager.DeleteProvider(name) })
			case "n", "N", "esc":
				m.mode = modeList
				return m, nil
			}
			return m, nil
		case modeTesting:
			// While testing, ignore all keys except cancel.
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		case modeAdd:
			switch msg.String() {
			case "esc":
				m.mode = modeList
				m.input.Blur()
				return m, nil
			case "enter":
				name := strings.TrimSpace(m.input.Value())
				m.mode = modeList
				m.input.Blur()
				m.input.SetValue("")
				if name == "" {
					return m, nil
				}
				return m, runOp("added "+name, func() error { return model.Add(name, "") })
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		case modeRename:
			switch msg.String() {
			case "esc":
				m.mode = modeList
				m.input.Blur()
				return m, nil
			case "enter":
				newName := strings.TrimSpace(m.input.Value())
				oldName := m.renameFrom
				m.mode = modeList
				m.input.Blur()
				m.input.SetValue("")
				if newName == "" || newName == oldName {
					return m, nil
				}
				return m, runOp(fmt.Sprintf("renamed %s → %s", oldName, newName), func() error {
					return manager.RenameProvider(oldName, newName)
				})
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "u", "enter":
			name := selectedName(m)
			if name == "" {
				return m, nil
			}
			return m, runOp("switched to "+name, func() error { return manager.SwitchProvider(name) })
		case "v", " ":
			name := selectedName(m)
			if name == "" {
				return m, nil
			}
			raw, err := manager.GetProviderRaw(name)
			if err != nil {
				m.err = err
				return m, nil
			}
			masked := ui.MaskSensitive(raw)
			m.viewport.SetContent(string(masked))
			m.mode = modeView
			return m, nil
		case "d":
			if selectedName(m) == "" {
				return m, nil
			}
			m.mode = modeConfirmDel
			return m, nil
		case "a":
			m.mode = modeAdd
			m.inputPrompt = "New model name (letters, digits, - _ .)"
			m.input.Placeholder = "e.g. my-model"
			m.input.SetValue("")
			m.input.Focus()
			return m, textinput.Blink
		case "n":
			name := selectedName(m)
			if name == "" {
				return m, nil
			}
			m.mode = modeRename
			m.renameFrom = name
			m.inputPrompt = fmt.Sprintf("Rename %q to", name)
			m.input.Placeholder = "new name"
			m.input.SetValue(name)
			m.input.Focus()
			return m, textinput.Blink
		case "e":
			name := selectedName(m)
			if name == "" {
				return m, nil
			}
			// Bubble Tea programs cannot be nested cleanly, so we quit and let
			// RunManager launch the editor before re-entering the manager loop.
			m.editRequest = name
			return m, tea.Quit
		case "t":
			name := selectedName(m)
			if name == "" {
				return m, nil
			}
			m.mode = modeTesting
			return m, m.testCmd([]string{name})
		case "T":
			names := allNames(m)
			if len(names) == 0 {
				return m, nil
			}
			m.mode = modeTesting
			return m, m.testCmd(names)
		case "r":
			return m, m.reloadCmd()
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m managerModel) View() string {
	var footer string
	switch m.mode {
	case modeConfirmDel:
		footer = errorStyle.Render(fmt.Sprintf("delete %q? y/n", selectedName(m)))
	case modeTesting:
		footer = m.spinner.View() + " " + fmt.Sprintf("testing %d/%d", m.testDone, m.testTotal)
	case modeView:
		return m.viewport.View() + "\n" + helpStyle.Render("esc/v/space to return")
	case modeAdd, modeRename:
		return m.list.View() + "\n" +
			titleStyle.Render(m.inputPrompt) + "\n" +
			m.input.View() + "\n" +
			helpStyle.Render("enter: confirm  •  esc: cancel")
	default:
		hints := "enter/u: use  •  a: add  •  n: rename  •  e: edit  •  t: test  •  T: all  •  v: view  •  d: delete  •  r: refresh  •  q: quit"
		footer = helpStyle.Render(hints)
	}
	body := m.list.View()
	status := ""
	if m.err != nil {
		status = errorStyle.Render(m.err.Error())
	} else if m.statusMsg != "" {
		status = statusOK(m.statusMsg)
	} else if m.editHint != "" {
		status = helpStyle.Render("returned from editing: " + m.editHint)
	}
	return body + "\n" + status + "\n" + footer
}

func selectedName(m managerModel) string {
	if it, ok := m.list.SelectedItem().(managerItem); ok {
		return it.name
	}
	return ""
}

func allNames(m managerModel) []string {
	items := m.list.Items()
	names := make([]string, 0, len(items))
	for _, it := range items {
		if mi, ok := it.(managerItem); ok {
			names = append(names, mi.name)
		}
	}
	return names
}

// runOp wraps a synchronous manager call so the UI can display success/error
// messages without blocking the event loop.
func runOp(successMsg string, op func() error) tea.Cmd {
	return func() tea.Msg {
		if err := op(); err != nil {
			return operationDoneMsg{err: err}
		}
		return operationDoneMsg{msg: successMsg}
	}
}

// reloadCmd rebuilds the model list from disk.
func (m managerModel) reloadCmd() tea.Cmd {
	return func() tea.Msg {
		names, err := manager.ListModels()
		if err != nil {
			return modelsReloadedMsg{err: err}
		}
		current, _ := manager.GetCurrentProvider()
		st, _ := tester.LoadStatus()
		return modelsReloadedMsg{items: buildManagerItems(names, current, st), current: current}
	}
}

// testCmd runs stream+tool checks against the given names.
func (m managerModel) testCmd(names []string) tea.Cmd {
	return func() tea.Msg {
		models := map[string]*model.Model{}
		for _, n := range names {
			mp, err := model.LoadByName(n)
			if err != nil {
				continue
			}
			models[n] = mp
		}
		if len(models) == 0 {
			return testResultsMsg{err: errors.New("no models could be loaded")}
		}
		client := tester.NewClient(tester.Options{})
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		results := client.TestAll(ctx, models, nil)
		return testResultsMsg{results: results}
	}
}
