package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cc-switch-cli/internal/mcp"
	"cc-switch-cli/internal/mcptest"
)

type mcpMode int

const (
	mcpModeList mcpMode = iota
	mcpModeView
	mcpModeTools
	mcpModeConfirmDel
	mcpModeTesting
	mcpModeAdd
	mcpModeRename
	mcpModeEdit
	mcpModeTestResults
)

type mcpItem struct {
	entry  mcp.Entry
	status *mcptest.Result
}

func (m mcpItem) Title() string {
	scope := fmt.Sprintf("[%s]", m.entry.Scope)
	name := m.entry.Name
	transport := string(m.entry.Server.Transport())
	suffix := ""
	if m.status != nil {
		switch m.status.Status {
		case mcptest.StatusOK:
			suffix = "  " + statusOK("OK")
			if m.status.ToolCount >= 0 {
				suffix += fmt.Sprintf(" (%d tools)", m.status.ToolCount)
			}
		case mcptest.StatusFail:
			suffix = "  " + statusFail("FAIL")
		case mcptest.StatusUnsupported:
			suffix = "  " + statusWarn("N/A")
		}
	}
	return fmt.Sprintf("%-9s %s  %s%s", scope, name, helpStyle.Render(transport), suffix)
}
func (m mcpItem) Description() string {
	return mcpEndpointLine(m.entry.Server)
}
func (m mcpItem) FilterValue() string {
	return m.entry.Name + " " + string(m.entry.Scope)
}

type mcpDelegate struct{}

func (mcpDelegate) Height() int                             { return 2 }
func (mcpDelegate) Spacing() int                            { return 0 }
func (mcpDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (mcpDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	it, ok := item.(mcpItem)
	if !ok {
		return
	}
	first := it.Title()
	second := helpStyle.Render(it.Description())
	if index == m.Index() {
		first = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true).Render("> " + strings.TrimLeft(first, " "))
	}
	fmt.Fprintf(w, "%s\n%s", first, second)
}

func mcpEndpointLine(s *mcp.Server) string {
	if s == nil {
		return ""
	}
	m := s.Masked()
	switch m.Transport() {
	case mcp.TransportStdio:
		parts := []string{m.Command}
		parts = append(parts, m.Args...)
		joined := strings.Join(parts, " ")
		if len(joined) > 70 {
			return joined[:67] + "..."
		}
		return joined
	case mcp.TransportHTTP, mcp.TransportSSE, mcp.TransportWS:
		if len(m.URL) > 70 {
			return m.URL[:67] + "..."
		}
		return m.URL
	default:
		return "(invalid config)"
	}
}

type mcpManagerModel struct {
	list        list.Model
	viewport    viewport.Model
	spinner     spinner.Model
	input       textinput.Model
	mode        mcpMode
	filterScope string // "", "user", "local", "project"
	projectDir  string
	statusMsg   string
	err         error
	testTotal   int
	testDone    int
	lastResults []mcptest.Result // most recent probe batch, reopenable with R
	// add/rename/edit form state
	inputPrompt string
	renameFrom  string
	renameScope mcp.Scope
	editName    string
	editScope   mcp.Scope
	addFields   []textinput.Model
	addLabels   []string
	addFocus    int
	// re-entry after editor
	editRequest string // "scope/name"
	editHint    string
}

type mcpOpDoneMsg struct {
	msg string
	err error
}
type mcpReloadedMsg struct {
	items []list.Item
	err   error
}
type mcpTestProgressMsg struct{ done, total int }
type mcpTestResultsMsg struct {
	results []mcptest.Result
	err     error
}

// RunMCPManager launches the interactive MCP manager.
func RunMCPManager() error {
	editRequest := ""
	for {
		m, err := newMCPManagerModel()
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
		fm, ok := final.(mcpManagerModel)
		if !ok {
			return nil
		}
		if fm.editRequest != "" {
			parts := strings.SplitN(fm.editRequest, "/", 2)
			if len(parts) == 2 {
				if err := RunMCPEditor(parts[0], parts[1], fm.projectDir); err != nil {
					return fmt.Errorf("edit %s: %w", fm.editRequest, err)
				}
			}
			editRequest = fm.editRequest
			continue
		}
		return nil
	}
}

func newMCPManagerModel() (mcpManagerModel, error) {
	dir, err := os.Getwd()
	if err != nil {
		return mcpManagerModel{}, err
	}
	entries, err := mcp.ListAll(dir)
	if err != nil {
		return mcpManagerModel{}, err
	}
	status, _ := mcptest.LoadStatus()
	items := buildMCPItems(entries, status)

	l := list.New(items, mcpDelegate{}, 80, 20)
	l.Title = "cc-switch-cli - MCP manager"
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

	return mcpManagerModel{
		list:       l,
		viewport:   vp,
		spinner:    sp,
		input:      ti,
		mode:       mcpModeList,
		projectDir: dir,
	}, nil
}

func buildMCPItems(entries []mcp.Entry, status *mcptest.StatusFile) []list.Item {
	items := make([]list.Item, 0, len(entries))
	for _, e := range entries {
		it := mcpItem{entry: e}
		if status != nil {
			if s, ok := status.Results[mcptest.CacheKey(e.Scope, e.Name)]; ok {
				sc := s
				it.status = &sc
			}
		}
		items = append(items, it)
	}
	return items
}

func (m mcpManagerModel) Init() tea.Cmd { return m.spinner.Tick }

func (m mcpManagerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height-2)
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 4
		if m.mode == mcpModeTestResults && len(m.lastResults) > 0 {
			m.viewport.SetContent(renderTestResults(
				summarizeMCP(m.lastResults),
				renderMCPTestResults(m.lastResults),
				msg.Width,
			))
		}
	case mcpOpDoneMsg:
		m.statusMsg = msg.msg
		m.err = msg.err
		return m, m.reloadCmd()
	case mcpReloadedMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.list.SetItems(msg.items)
	case mcpTestProgressMsg:
		m.testDone = msg.done
		m.testTotal = msg.total
	case mcpTestResultsMsg:
		if msg.err != nil {
			m.mode = mcpModeList
			m.err = msg.err
			return m, nil
		}
		st, _ := mcptest.LoadStatus()
		if st == nil {
			st = &mcptest.StatusFile{Results: map[string]mcptest.Result{}}
		}
		st.Upsert(msg.results)
		_ = mcptest.SaveStatus(st)
		// Show the full probe results — errors included — instead of dropping
		// the user back on a bare list with no explanation of a FAIL.
		m.mode = mcpModeTestResults
		m.lastResults = msg.results
		m.err = nil
		m.statusMsg = ""
		m.viewport.GotoTop()
		m.viewport.SetContent(renderTestResults(
			summarizeMCP(msg.results),
			renderMCPTestResults(msg.results),
			m.viewport.Width,
		))
		return m, m.reloadCmd()
	case mcpToolsContentMsg:
		m.viewport.SetContent(msg.content)
		m.mode = mcpModeTools
		return m, nil
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
		case mcpModeView, mcpModeTools, mcpModeTestResults:
			switch msg.String() {
			case "esc", "q", "v", " ", "l":
				m.mode = mcpModeList
				return m, nil
			}
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		case mcpModeConfirmDel:
			switch msg.String() {
			case "y", "Y":
				e := selectedMCP(m)
				m.mode = mcpModeList
				if e == nil {
					return m, nil
				}
				return m, runMCPOp("removed "+e.entry.Name, func() error {
					return mcp.Remove(e.entry.Scope, m.projectDir, e.entry.Name)
				})
			case "n", "N", "esc":
				m.mode = mcpModeList
				return m, nil
			}
			return m, nil
		case mcpModeTesting:
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		case mcpModeRename:
			switch msg.String() {
			case "esc":
				m.mode = mcpModeList
				m.input.Blur()
				return m, nil
			case "enter":
				newName := strings.TrimSpace(m.input.Value())
				old := m.renameFrom
				sc := m.renameScope
				m.mode = mcpModeList
				m.input.Blur()
				m.input.SetValue("")
				if newName == "" || newName == old {
					return m, nil
				}
				return m, runMCPOp(fmt.Sprintf("renamed %s -> %s", old, newName), func() error {
					srv, err := mcp.Get(sc, m.projectDir, old)
					if err != nil {
						return err
					}
					if err := mcp.Set(sc, m.projectDir, newName, srv); err != nil {
						return err
					}
					return mcp.Remove(sc, m.projectDir, old)
				})
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		case mcpModeAdd:
			return m.updateAdd(msg)
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "enter", "l":
			e := selectedMCP(m)
			if e == nil {
				return m, nil
			}
			return m, m.toolsCmd(e.entry)
		case "v", " ":
			e := selectedMCP(m)
			if e == nil {
				return m, nil
			}
			out, err := e.entry.Server.Masked().Indent()
			if err != nil {
				m.err = err
				return m, nil
			}
			header := fmt.Sprintf("# scope=%s name=%s transport=%s\n", e.entry.Scope, e.entry.Name, e.entry.Server.Transport())
			m.viewport.SetContent(header + string(out))
			m.mode = mcpModeView
			return m, nil
		case "d":
			if selectedMCP(m) == nil {
				return m, nil
			}
			m.mode = mcpModeConfirmDel
			return m, nil
		case "a":
			return m, m.startAdd()
		case "n":
			e := selectedMCP(m)
			if e == nil {
				return m, nil
			}
			m.mode = mcpModeRename
			m.renameFrom = e.entry.Name
			m.renameScope = e.entry.Scope
			m.inputPrompt = fmt.Sprintf("Rename %q to", e.entry.Name)
			m.input.Placeholder = "new name"
			m.input.SetValue(e.entry.Name)
			m.input.Focus()
			return m, textinput.Blink
		case "e":
			e := selectedMCP(m)
			if e == nil {
				return m, nil
			}
			m.editRequest = string(e.entry.Scope) + "/" + e.entry.Name
			return m, tea.Quit
		case "t":
			e := selectedMCP(m)
			if e == nil {
				return m, nil
			}
			m.mode = mcpModeTesting
			return m, m.testCmd([]mcp.Entry{e.entry})
		case "T":
			entries := allMCPEntries(m)
			if len(entries) == 0 {
				return m, nil
			}
			m.mode = mcpModeTesting
			return m, m.testCmd(entries)
		case "R":
			if len(m.lastResults) == 0 {
				return m, nil
			}
			m.mode = mcpModeTestResults
			m.viewport.GotoTop()
			m.viewport.SetContent(renderTestResults(
				summarizeMCP(m.lastResults),
				renderMCPTestResults(m.lastResults),
				m.viewport.Width,
			))
			return m, nil
		case "s":
			m.cycleScopeFilter()
			return m, m.reloadCmd()
		case "r":
			return m, m.reloadCmd()
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *mcpManagerModel) cycleScopeFilter() {
	switch m.filterScope {
	case "":
		m.filterScope = "user"
	case "user":
		m.filterScope = "local"
	case "local":
		m.filterScope = "project"
	default:
		m.filterScope = ""
	}
}

func (m mcpManagerModel) View() string {
	var footer string
	switch m.mode {
	case mcpModeConfirmDel:
		e := selectedMCP(m)
		name := ""
		if e != nil {
			name = e.entry.Name
		}
		footer = errorStyle.Render(fmt.Sprintf("delete %q? y/n", name))
	case mcpModeTesting:
		footer = m.spinner.View() + " " + fmt.Sprintf("testing %d/%d", m.testDone, m.testTotal)
	case mcpModeTestResults:
		return m.viewport.View() + "\n" + helpStyle.Render("esc/l: back • j/k, arrows, pgup/pgdn: scroll")
	case mcpModeView:
		return m.viewport.View() + "\n" + helpStyle.Render("esc/v/space to return")
	case mcpModeTools:
		return m.viewport.View() + "\n" + helpStyle.Render("esc/l to return")
	case mcpModeRename:
		return m.list.View() + "\n" +
			titleStyle.Render(m.inputPrompt) + "\n" +
			m.input.View() + "\n" +
			helpStyle.Render("enter: confirm  |  esc: cancel")
	case mcpModeAdd:
		return m.viewAdd()
	default:
		scopeHint := "all"
		if m.filterScope != "" {
			scopeHint = m.filterScope
		}
		hints := fmt.Sprintf("enter/l: tools  |  a: add  |  e: edit  |  n: rename  |  t: test  |  T: all  |  R: last results  |  v: view  |  d: delete  |  s: scope(%s)  |  r: refresh  |  q: quit", scopeHint)
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

func selectedMCP(m mcpManagerModel) *mcpItem {
	if it, ok := m.list.SelectedItem().(mcpItem); ok {
		return &it
	}
	return nil
}

func allMCPEntries(m mcpManagerModel) []mcp.Entry {
	items := m.list.Items()
	out := make([]mcp.Entry, 0, len(items))
	for _, it := range items {
		if mi, ok := it.(mcpItem); ok {
			out = append(out, mi.entry)
		}
	}
	return out
}

func runMCPOp(successMsg string, op func() error) tea.Cmd {
	return func() tea.Msg {
		if err := op(); err != nil {
			return mcpOpDoneMsg{err: err}
		}
		return mcpOpDoneMsg{msg: successMsg}
	}
}

func (m mcpManagerModel) reloadCmd() tea.Cmd {
	filter := m.filterScope
	dir := m.projectDir
	return func() tea.Msg {
		var (
			entries []mcp.Entry
			err     error
		)
		if filter == "" {
			entries, err = mcp.ListAll(dir)
		} else {
			sc, perr := mcp.ParseScope(filter)
			if perr != nil {
				return mcpReloadedMsg{err: perr}
			}
			entries, err = mcp.List(sc, dir)
		}
		if err != nil {
			return mcpReloadedMsg{err: err}
		}
		st, _ := mcptest.LoadStatus()
		return mcpReloadedMsg{items: buildMCPItems(entries, st)}
	}
}

func (m mcpManagerModel) testCmd(entries []mcp.Entry) tea.Cmd {
	dir := m.projectDir
	return func() tea.Msg {
		client := mcptest.Options{
			Timeout:     30 * time.Second,
			Concurrency: 4,
			ProjectDir:  dir,
			WithTools:   true,
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		var progress func(done, total int)
		// progress is reported via separate messages in a full design; keep simple.
		_ = progress
		results := mcptest.ProbeAll(ctx, entries, client, nil)
		return mcpTestResultsMsg{results: results}
	}
}

func (m mcpManagerModel) toolsCmd(e mcp.Entry) tea.Cmd {
	dir := m.projectDir
	return func() tea.Msg {
		r := mcptest.Probe(context.Background(), e, mcptest.Options{
			Timeout:    30 * time.Second,
			ProjectDir: dir,
			WithTools:  true,
		})
		var b strings.Builder
		fmt.Fprintf(&b, "# %s (%s) - tools\n\n", e.Name, e.Server.Transport())
		if r.Status != mcptest.StatusOK {
			fmt.Fprintf(&b, "probe failed: %s\n%s\n", r.Status, r.Error)
		} else if r.ToolCount < 0 {
			fmt.Fprintf(&b, "reachable, but tools/list failed: %s\n", r.Error)
		} else if r.ToolCount == 0 {
			b.WriteString("(no tools)\n")
		} else {
			for _, t := range r.Tools {
				desc := t.Description
				if len(desc) > 100 {
					desc = desc[:97] + "..."
				}
				if desc == "" {
					fmt.Fprintf(&b, "  %s\n", t.Name)
				} else {
					fmt.Fprintf(&b, "  %-32s %s\n", t.Name, desc)
				}
			}
		}
		// Return via operation path by mutating is awkward; use a dedicated msg.
		return mcpToolsContentMsg{content: b.String()}
	}
}

type mcpToolsContentMsg struct{ content string }

// extend Update to handle tools content ...re-open Update for this msg via
// a small patch: handle in the main switch by editing the function... done below
// by making toolsCmd set mode via a combined approach.

func (m *mcpManagerModel) startAdd() tea.Cmd {
	m.mode = mcpModeAdd
	m.addLabels = []string{
		"name",
		"scope (user|local|project)",
		"transport (stdio|http|sse)",
		"command (stdio)",
		"args (space-separated)",
		"url (http/sse)",
		"env (KEY=VAL space-separated)",
		"headers (Name: value; ...)",
	}
	m.addFields = make([]textinput.Model, len(m.addLabels))
	for i, label := range m.addLabels {
		ti := textinput.New()
		ti.CharLimit = 512
		ti.Width = 56
		ti.Prompt = "  "
		switch i {
		case 1:
			ti.SetValue("local")
		case 2:
			ti.SetValue("stdio")
		}
		_ = label
		m.addFields[i] = ti
	}
	m.addFocus = 0
	m.addFields[0].Focus()
	return textinput.Blink
}

func (m mcpManagerModel) updateAdd(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = mcpModeList
		return m, nil
	case "tab", "down":
		m.addFields[m.addFocus].Blur()
		m.addFocus = (m.addFocus + 1) % len(m.addFields)
		m.addFields[m.addFocus].Focus()
		return m, textinput.Blink
	case "shift+tab", "up":
		m.addFields[m.addFocus].Blur()
		m.addFocus = (m.addFocus - 1 + len(m.addFields)) % len(m.addFields)
		m.addFields[m.addFocus].Focus()
		return m, textinput.Blink
	case "ctrl+s", "enter":
		if msg.String() == "enter" && m.addFocus < len(m.addFields)-1 {
			m.addFields[m.addFocus].Blur()
			m.addFocus++
			m.addFields[m.addFocus].Focus()
			return m, textinput.Blink
		}
		srv, name, scope, err := m.parseAddForm()
		m.mode = mcpModeList
		if err != nil {
			m.err = err
			return m, nil
		}
		dir := m.projectDir
		return m, runMCPOp("added "+name, func() error {
			return mcp.Set(scope, dir, name, srv)
		})
	}
	var cmd tea.Cmd
	m.addFields[m.addFocus], cmd = m.addFields[m.addFocus].Update(msg)
	return m, cmd
}

func (m mcpManagerModel) parseAddForm() (*mcp.Server, string, mcp.Scope, error) {
	val := func(i int) string { return strings.TrimSpace(m.addFields[i].Value()) }
	name := val(0)
	if err := mcp.ValidateName(name); err != nil {
		return nil, "", "", err
	}
	scope, err := mcp.ParseScope(val(1))
	if err != nil {
		return nil, "", "", err
	}
	transport := strings.ToLower(val(2))
	srv := &mcp.Server{}
	switch transport {
	case "stdio", "":
		srv.Command = val(3)
		if args := val(4); args != "" {
			srv.Args = strings.Fields(args)
		}
		if env := val(6); env != "" {
			srv.Env = map[string]string{}
			for _, pair := range strings.Fields(env) {
				k, v, ok := strings.Cut(pair, "=")
				if ok {
					srv.Env[k] = v
				}
			}
		}
	case "http", "sse":
		srv.Type = transport
		srv.URL = val(5)
		if hdrs := val(7); hdrs != "" {
			srv.Headers = map[string]string{}
			for _, part := range strings.Split(hdrs, ";") {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				k, v, ok := strings.Cut(part, ":")
				if ok {
					srv.Headers[strings.TrimSpace(k)] = strings.TrimSpace(v)
				}
			}
		}
	default:
		return nil, "", "", fmt.Errorf("unknown transport %q", transport)
	}
	if err := srv.Validate(); err != nil {
		return nil, "", "", err
	}
	return srv, name, scope, nil
}

func (m mcpManagerModel) viewAdd() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Add MCP server"))
	b.WriteString("\n\n")
	for i, label := range m.addLabels {
		ls := lipgloss.NewStyle().Width(32)
		if i == m.addFocus {
			ls = ls.Foreground(lipgloss.Color("212")).Bold(true)
		}
		b.WriteString(ls.Render(label))
		b.WriteString(m.addFields[i].View())
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	b.WriteString(helpStyle.Render("tab: next  |  ctrl+s: save  |  esc: cancel"))
	return b.String()
}
