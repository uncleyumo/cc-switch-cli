package tui

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cc-switch-cli/internal/model"
)

// editorField describes one row in the structured editor.
type editorField struct {
	label   string
	envKey  string // "" for non-env fields
	restKey string // "" for env fields
	input   textinput.Model
	secret  bool
}

const fieldContextWindow = "context-window"

// editorMode selects between the structured form and the raw JSON textarea.
type editorMode int

const (
	editStructured editorMode = iota
	editRaw
)

// editorModel is the Bubble Tea model for `model edit`.
type editorModel struct {
	name    string
	fields  []editorField
	focused int
	model   *model.Model
	mode    editorMode
	raw     textarea.Model
	err     error
	saved   bool
	quit    bool
	status  string
}

// RunEditor loads a provider and opens the structured editor. Save on ctrl+s,
// cancel with esc.
func RunEditor(name string) error {
	m, err := model.LoadByName(name)
	if err != nil {
		return err
	}
	em, err := newEditorModel(name, m)
	if err != nil {
		return err
	}
	p := tea.NewProgram(em)
	final, err := p.Run()
	if err != nil {
		return err
	}
	if fm, ok := final.(editorModel); ok {
		if fm.err != nil {
			return fm.err
		}
	}
	return nil
}

func newEditorModel(name string, m *model.Model) (editorModel, error) {
	fields := []editorField{
		{label: "ANTHROPIC_AUTH_TOKEN", envKey: model.EnvAuthToken, secret: true, input: newInput(m.Env.AuthToken, true)},
		{label: "ANTHROPIC_API_KEY", envKey: model.EnvAPIKey, secret: true, input: newInput(m.Env.APIKey, true)},
		{label: "ANTHROPIC_BASE_URL", envKey: model.EnvBaseURL, input: newInput(m.Env.BaseURL, false)},
		{label: "ANTHROPIC_MODEL", envKey: model.EnvModel, input: newInput(m.Env.Model, false)},
		{label: "ANTHROPIC_DEFAULT_FABLE_MODEL", envKey: model.EnvDefaultFableModel, input: newInput(m.Env.DefaultFableModel, false)},
		{label: "ANTHROPIC_DEFAULT_HAIKU_MODEL", envKey: model.EnvDefaultHaikuModel, input: newInput(m.Env.DefaultHaikuModel, false)},
		{label: "ANTHROPIC_DEFAULT_OPUS_MODEL", envKey: model.EnvDefaultOpusModel, input: newInput(m.Env.DefaultOpusModel, false)},
		{label: "ANTHROPIC_DEFAULT_SONNET_MODEL", envKey: model.EnvDefaultSonnetModel, input: newInput(m.Env.DefaultSonnetModel, false)},
		{label: "model (top-level)", restKey: "model", input: newInput(m.TopLevelModel(), false)},
		{label: "context window", envKey: fieldContextWindow, input: newInput(model.FormatContextWindow(m.Env.ContextWindowTokens()), false)},
	}
	fields[0].input.Focus()

	ta := textarea.New()
	ta.SetHeight(20)
	ta.SetWidth(80)
	initial, err := m.Indent()
	if err != nil {
		return editorModel{}, err
	}
	ta.SetValue(string(initial))
	ta.Blur()

	return editorModel{name: name, fields: fields, model: m, raw: ta}, nil
}

func newInput(value string, secret bool) textinput.Model {
	ti := textinput.New()
	ti.Prompt = "  "
	ti.CharLimit = 512
	ti.Width = 60
	ti.SetValue(value)
	if secret {
		ti.EchoMode = textinput.EchoPassword
		ti.EchoCharacter = '•'
	}
	return ti
}

func (m editorModel) Init() tea.Cmd { return textinput.Blink }

func (m editorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			// esc leaves raw mode without quitting; from structured it cancels.
			if m.mode == editRaw {
				m.mode = editStructured
				m.raw.Blur()
				m.fields[m.focused].input.Focus()
				m.status = ""
				return m, nil
			}
			m.quit = true
			return m, tea.Quit
		case "ctrl+j":
			// Toggle between structured and raw JSON editing. Structured →
			// raw dumps the current form state; raw → structured re-parses
			// so field edits stay in sync.
			if m.mode == editStructured {
				if err := m.syncStructuredToRaw(); err != nil {
					m.err = err
					m.status = "cannot switch to JSON: " + err.Error()
					return m, nil
				}
				m.mode = editRaw
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
				m.mode = editStructured
				m.raw.Blur()
				m.fields[m.focused].input.Focus()
				m.status = ""
			}
			m.err = nil
			return m, nil
		case "ctrl+s":
			if m.mode == editRaw {
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
			m.saved = true
			m.status = "saved."
			return m, tea.Quit
		case "tab", "down":
			if m.mode == editStructured {
				m.focused = (m.focused + 1) % len(m.fields)
				m.refocus()
				return m, nil
			}
		case "shift+tab", "up":
			if m.mode == editStructured {
				m.focused = (m.focused - 1 + len(m.fields)) % len(m.fields)
				m.refocus()
				return m, nil
			}
		}
	}
	if m.mode == editRaw {
		var cmd tea.Cmd
		m.raw, cmd = m.raw.Update(msg)
		return m, cmd
	}
	var cmd tea.Cmd
	m.fields[m.focused].input, cmd = m.fields[m.focused].input.Update(msg)
	return m, cmd
}

func (m *editorModel) refocus() {
	for i := range m.fields {
		if i == m.focused {
			m.fields[i].input.Focus()
		} else {
			m.fields[i].input.Blur()
		}
	}
}

// syncStructuredToRaw takes the current form values and updates the model +
// textarea so the raw editor starts from what the user has entered so far.
func (m *editorModel) syncStructuredToRaw() error {
	if err := m.applyFieldsToModel(); err != nil {
		return err
	}
	out, err := m.model.Indent()
	if err != nil {
		return err
	}
	m.raw.SetValue(string(out))
	return nil
}

// syncRawToStructured parses the textarea, replaces the in-memory model and
// re-populates the form fields so subsequent structured edits are consistent.
func (m *editorModel) syncRawToStructured() error {
	parsed, err := model.Parse([]byte(m.raw.Value()))
	if err != nil {
		return err
	}
	parsed.Name = m.name
	m.model = parsed
	m.fields[0].input.SetValue(parsed.Env.AuthToken)
	m.fields[1].input.SetValue(parsed.Env.APIKey)
	m.fields[2].input.SetValue(parsed.Env.BaseURL)
	m.fields[3].input.SetValue(parsed.Env.Model)
	m.fields[4].input.SetValue(parsed.Env.DefaultFableModel)
	m.fields[5].input.SetValue(parsed.Env.DefaultHaikuModel)
	m.fields[6].input.SetValue(parsed.Env.DefaultOpusModel)
	m.fields[7].input.SetValue(parsed.Env.DefaultSonnetModel)
	m.fields[8].input.SetValue(parsed.TopLevelModel())
	m.fields[9].input.SetValue(model.FormatContextWindow(parsed.Env.ContextWindowTokens()))
	return nil
}

// applyFieldsToModel copies form field values into the in-memory model.
func (m *editorModel) applyFieldsToModel() error {
	env := &m.model.Env
	if m.model.Rest == nil {
		m.model.Rest = map[string]json.RawMessage{}
	}
	for _, f := range m.fields {
		val := strings.TrimSpace(f.input.Value())
		switch f.envKey {
		case model.EnvAuthToken:
			env.AuthToken = val
		case model.EnvAPIKey:
			env.APIKey = val
		case model.EnvBaseURL:
			env.BaseURL = val
		case model.EnvModel:
			env.Model = val
		case model.EnvDefaultFableModel:
			env.DefaultFableModel = val
		case model.EnvDefaultHaikuModel:
			env.DefaultHaikuModel = val
		case model.EnvDefaultOpusModel:
			env.DefaultOpusModel = val
		case model.EnvDefaultSonnetModel:
			env.DefaultSonnetModel = val
		case fieldContextWindow:
			n, err := model.ParseContextWindow(val)
			if err != nil {
				return err
			}
			m.model.ApplyContextWindow(n)
		}
		if f.restKey == "model" {
			if val == "" {
				delete(m.model.Rest, "model")
			} else if enc, err := jsonQuote(val); err == nil {
				m.model.Rest["model"] = enc
			}
		}
	}
	return nil
}

func (m *editorModel) applyAndSave() error {
	if m.model == nil {
		return errors.New("no model loaded")
	}
	if m.mode == editStructured {
		if err := m.applyFieldsToModel(); err != nil {
			return err
		}
	}
	if err := model.Validate(m.model); err != nil {
		return err
	}
	return model.Save(m.name, m.model)
}

// topLevelState returns the live ANTHROPIC_MODEL and top-level `model` form
// values, so the view can explain which one will actually win.
func (m editorModel) topLevelState() (envModel, topLevel string) {
	for _, f := range m.fields {
		switch {
		case f.envKey == model.EnvModel:
			envModel = strings.TrimSpace(f.input.Value())
		case f.restKey == "model":
			topLevel = strings.TrimSpace(f.input.Value())
		}
	}
	return envModel, topLevel
}

// topLevelModelHint explains what the top-level `model` field does in the
// current form state. The warn result is true when ANTHROPIC_MODEL is also set,
// because Claude Code gives the env var priority at startup.
func topLevelModelHint(envModel, topLevel string) (hint string, warn bool) {
	switch {
	case envModel != "" && topLevel != "":
		return "overridden by ANTHROPIC_MODEL; leave this empty when using the env var", true
	case topLevel != "":
		return "used only when ANTHROPIC_MODEL is empty (alias or full model id)", false
	case envModel != "":
		return "optional; ANTHROPIC_MODEL is already set, so this can stay empty", false
	default:
		return "optional; without it Claude Code falls back to ANTHROPIC_MODEL or its default model", false
	}
}

func (m editorModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("Edit model: %s", m.name)))
	b.WriteString("\n\n")

	if m.mode == editRaw {
		b.WriteString(m.raw.View())
		b.WriteByte('\n')
	} else {
		envModel, topLevel := m.topLevelState()
		overridden := envModel != "" && topLevel != ""
		for i, f := range m.fields {
			labelStyle := lipgloss.NewStyle().Width(34)
			if i == m.focused {
				labelStyle = labelStyle.Foreground(lipgloss.Color("212")).Bold(true)
			}
			label := f.label
			if f.restKey == "model" && overridden {
				label += " (overridden)"
			}
			b.WriteString(labelStyle.Render(label))
			b.WriteString(f.input.View())
			b.WriteByte('\n')
			if i == m.focused && f.restKey == "model" {
				hint, warn := topLevelModelHint(envModel, topLevel)
				if warn {
					b.WriteString("  " + statusWarn(hint))
				} else {
					b.WriteString("  " + helpStyle.Render(hint))
				}
				b.WriteByte('\n')
			}
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
	hints := "tab: next  •  shift+tab: prev  •  ctrl+j: JSON mode  •  ctrl+s: save  •  esc: cancel"
	if m.mode == editRaw {
		hints = "ctrl+j: back to fields  •  ctrl+s: save  •  esc: cancel"
	} else {
		hints += "\ncontext window: 128k / 256k / 500k / 1m / 2m (empty = Claude Code default)"
	}
	b.WriteString(helpStyle.Render(hints))
	return b.String()
}
