package tui

import (
	"strings"
	"testing"

	"cc-switch-cli/internal/model"
)

func TestTopLevelModelHint(t *testing.T) {
	tests := []struct {
		name     string
		envModel string
		topLevel string
		wantWarn bool
		wantPart string
	}{
		{"both set", "opus", "sonnet", true, "overridden by ANTHROPIC_MODEL"},
		{"top only", "", "sonnet", false, "used only when ANTHROPIC_MODEL is empty"},
		{"env only", "opus", "", false, "can stay empty"},
		{"neither", "", "", false, "falls back to ANTHROPIC_MODEL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hint, warn := topLevelModelHint(tt.envModel, tt.topLevel)
			if warn != tt.wantWarn {
				t.Errorf("warn = %v, want %v", warn, tt.wantWarn)
			}
			if !strings.Contains(hint, tt.wantPart) {
				t.Errorf("hint = %q, want substring %q", hint, tt.wantPart)
			}
		})
	}
}

func TestEditorMarksOverriddenTopLevelModel(t *testing.T) {
	m := editorModel{
		fields: []editorField{
			{label: "ANTHROPIC_MODEL", envKey: model.EnvModel, input: newInput("opus", false)},
			{label: "model (top-level)", restKey: "model", input: newInput("sonnet", false)},
		},
		focused: 1,
	}
	out := m.View()
	if !strings.Contains(out, "(overridden)") {
		t.Errorf("top-level field should be marked as overridden:\n%s", out)
	}
	if !strings.Contains(out, "overridden by ANTHROPIC_MODEL") {
		t.Errorf("focused field should explain the override:\n%s", out)
	}
}
