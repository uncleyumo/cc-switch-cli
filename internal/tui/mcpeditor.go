package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cc-switch-cli/internal/mcp"
)

type mcpEditorMode int

const (
	mcpEditStructured mcpEditorMode = iota
	mcpEditRaw
)

type mcpEditorModel struct {
	scope      mcp.Scope
	name       string
	projectDir string
	server     *mcp.Server
	fields     []editorField
	focused    int
	mode       mcpEditorMode
	raw        textarea.Model
	err        error
	status     string
	quit       bool
}

// RunMCPEditor opens a structured + raw-JSON editor for one MCP server entry.
func RunMCPEditor(scopeStr, name, projectDir string) error {
	sc, err := mcp.ParseScope(scopeStr)
	if err != nil {
		return err
	}
	srv, err := mcp.Get(sc, projectDir, name)
	if err != nil {
		return err
	}
	em, err := newMCPEditorModel(sc, name, projectDir, srv)
	if err != nil {
		return err
	}
	p := tea.NewProgram(em)
	final, err := p.Run()
	if err != nil {
		return err
	}
	if fm, ok := final.(mcpEditorModel); ok && fm.err != nil {
		return fm.err
	}
	return nil
}

func newMCPEditorModel(scope mcp.Scope, name, projectDir string, srv *mcp.Server) (mcpEditorModel, error) {
	transport := string(srv.Transport())
	if transport == string(mcp.TransportInvalid) {
		transport = srv.Type
	}
	args := strings.Join(srv.Args, " ")
	env := joinMap(srv.Env, "=")
	headers := joinHeaders(srv.Headers)

	fields := []editorField{
		{label: "transport", input: newInput(transport, false)},
		{label: "command", input: newInput(srv.Command, false)},
		{label: "args", input: newInput(args, false)},
		{label: "url", input: newInput(srv.URL, false)},
		{label: "env (KEY=VAL ...)", input: newInput(env, false)},
		{label: "headers (Name: val; ...)", input: newInput(headers, false)},
	}
	fields[0].input.Focus()

	ta := textarea.New()
	ta.SetHeight(20)
	ta.SetWidth(80)
	initial, err := srv.Indent()
	if err != nil {
		return mcpEditorModel{}, err
	}
	ta.SetValue(string(initial))
	ta.Blur()

	return mcpEditorModel{
		scope:      scope,
		name:       name,
		projectDir: projectDir,
		server:     srv,
		fields:     fields,
		raw:        ta,
	}, nil
}

func joinMap(m map[string]string, sep string) string {
	if len(m) == 0 {
		return ""
	}
	parts := make([]string, 0, len(m))
	for k, v := range m {
		parts = append(parts, k+sep+v)
	}
	return strings.Join(parts, " ")
}

func joinHeaders(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	parts := make([]string, 0, len(m))
	for k, v := range m {
		parts = append(parts, k+": "+v)
	}
	return strings.Join(parts, "; ")
}

func (m mcpEditorModel) Init() tea.Cmd { return textinput.Blink }

func (m mcpEditorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.raw.SetWidth(msg.Width - 4)
		m.raw.SetHeight(msg.Height - 6)
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quit = true
			return m, tea.Quit
		case "esc":
			if m.mode == mcpEditRaw {
				m.mode = mcpEditStructured
				m.raw.Blur()
				m.fields[m.focused].input.Focus()
				m.status = ""
				return m, nil
			}
			m.quit = true
			return m, tea.Quit
		case "ctrl+j":
			if m.mode == mcpEditStructured {
				if err := m.syncStructuredToRaw(); err != nil {
					m.err = err
					m.status = "cannot switch to JSON: " + err.Error()
					return m, nil
				}
				m.mode = mcpEditRaw
				m.raw.Focus()
				for i := range m.fields {
					m.fields[i].input.Blur()
				}
				m.status = "JSON edit mode (esc to return, ctrl+s to save)"
			} else {
				if err := m.syncRawToStructured(); err != nil {
					m.err = err
					m.status = "invalid JSON: " + err.Error()
					return m, nil
				}
				m.mode = mcpEditStructured
				m.raw.Blur()
				m.fields[m.focused].input.Focus()
				m.status = ""
			}
			m.err = nil
			return m, nil
		case "ctrl+s":
			if m.mode == mcpEditRaw {
				if err := m.syncRawToStructured(); err != nil {
					m.err = err
					m.status = "invalid JSON: " + err.Error()
					return m, nil
				}
			}
			if err := m.applyAndSave(); err != nil {
				m.err = err
				m.status = "save failed: " + err.Error()
				return m, nil
			}
			m.status = "saved."
			return m, tea.Quit
		case "tab", "down":
			if m.mode == mcpEditStructured {
				m.focused = (m.focused + 1) % len(m.fields)
				m.refocus()
				return m, nil
			}
		case "shift+tab", "up":
			if m.mode == mcpEditStructured {
				m.focused = (m.focused - 1 + len(m.fields)) % len(m.fields)
				m.refocus()
				return m, nil
			}
		}
	}
	if m.mode == mcpEditRaw {
		var cmd tea.Cmd
		m.raw, cmd = m.raw.Update(msg)
		return m, cmd
	}
	var cmd tea.Cmd
	m.fields[m.focused].input, cmd = m.fields[m.focused].input.Update(msg)
	return m, cmd
}

func (m *mcpEditorModel) refocus() {
	for i := range m.fields {
		if i == m.focused {
			m.fields[i].input.Focus()
		} else {
			m.fields[i].input.Blur()
		}
	}
}

func (m *mcpEditorModel) syncStructuredToRaw() error {
	if err := m.applyFields(); err != nil {
		return err
	}
	out, err := m.server.Indent()
	if err != nil {
		return err
	}
	m.raw.SetValue(string(out))
	return nil
}

func (m *mcpEditorModel) syncRawToStructured() error {
	parsed, err := mcp.ParseServer([]byte(m.raw.Value()))
	if err != nil {
		return err
	}
	m.server = parsed
	m.fields[0].input.SetValue(string(parsed.Transport()))
	if parsed.Type != "" {
		m.fields[0].input.SetValue(parsed.Type)
	}
	m.fields[1].input.SetValue(parsed.Command)
	m.fields[2].input.SetValue(strings.Join(parsed.Args, " "))
	m.fields[3].input.SetValue(parsed.URL)
	m.fields[4].input.SetValue(joinMap(parsed.Env, "="))
	m.fields[5].input.SetValue(joinHeaders(parsed.Headers))
	return nil
}

func (m *mcpEditorModel) applyFields() error {
	transport := strings.ToLower(strings.TrimSpace(m.fields[0].input.Value()))
	command := strings.TrimSpace(m.fields[1].input.Value())
	argsStr := strings.TrimSpace(m.fields[2].input.Value())
	url := strings.TrimSpace(m.fields[3].input.Value())
	envStr := strings.TrimSpace(m.fields[4].input.Value())
	hdrStr := strings.TrimSpace(m.fields[5].input.Value())

	// Preserve Rest from the existing server.
	rest := m.server.Rest
	srv := &mcp.Server{Rest: rest}

	switch transport {
	case "stdio", "":
		srv.Command = command
		if argsStr != "" {
			srv.Args = strings.Fields(argsStr)
		}
		if envStr != "" {
			srv.Env = map[string]string{}
			for _, pair := range strings.Fields(envStr) {
				k, v, ok := strings.Cut(pair, "=")
				if ok {
					srv.Env[k] = v
				}
			}
		}
	case "http", "sse", "ws", "streamable-http":
		srv.Type = transport
		srv.URL = url
		if hdrStr != "" {
			srv.Headers = map[string]string{}
			for _, part := range strings.Split(hdrStr, ";") {
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
		return fmt.Errorf("unknown transport %q", transport)
	}
	srv.NormalizeType()
	if err := srv.Validate(); err != nil {
		return err
	}
	m.server = srv
	return nil
}

func (m *mcpEditorModel) applyAndSave() error {
	if m.mode == mcpEditStructured {
		if err := m.applyFields(); err != nil {
			return err
		}
	}
	return mcp.Set(m.scope, m.projectDir, m.name, m.server)
}

func (m mcpEditorModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("Edit MCP: %s [%s]", m.name, m.scope)))
	b.WriteString("\n\n")

	if m.mode == mcpEditRaw {
		b.WriteString(m.raw.View())
		b.WriteByte('\n')
	} else {
		for i, f := range m.fields {
			labelStyle := lipgloss.NewStyle().Width(28)
			if i == m.focused {
				labelStyle = labelStyle.Foreground(lipgloss.Color("212")).Bold(true)
			}
			b.WriteString(labelStyle.Render(f.label))
			b.WriteString(f.input.View())
			b.WriteByte('\n')
		}
	}

	b.WriteByte('\n')
	if m.status != "" {
		if m.err != nil {
			b.WriteString(errorStyle.Render(m.status))
		} else {
			b.WriteString(statusOK(m.status))
		}
		b.WriteByte('\n')
	}
	hints := "tab: next  •  ctrl+j: JSON mode  •  ctrl+s: save  •  esc: cancel"
	if m.mode == mcpEditRaw {
		hints = "ctrl+j: back to fields  •  ctrl+s: save  •  esc: cancel"
	}
	b.WriteString(helpStyle.Render(hints))
	return b.String()
}
