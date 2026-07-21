package tester

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestToolProbeDoesNotForceToolChoice guards the fix for the false "tool failed"
// verdicts: the tool probe must mirror Claude Code and leave tool_choice unset
// (API default "auto"), because forced values like "any" are rejected by several
// Anthropic-compatible gateways that otherwise relay tool_use just fine.
func TestToolProbeDoesNotForceToolChoice(t *testing.T) {
	req := buildToolReq("some-model", 512)
	if req.ToolChoice != nil {
		t.Fatalf("tool probe must not set tool_choice, got %+v", req.ToolChoice)
	}
	if len(req.Tools) != 1 {
		t.Fatalf("tool probe must declare exactly one tool, got %d", len(req.Tools))
	}
	if !req.Stream {
		t.Fatalf("tool probe must stream")
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "tool_choice") {
		t.Fatalf("serialized tool probe must not contain tool_choice: %s", string(raw))
	}
}

func TestStreamProbeHasNoTools(t *testing.T) {
	req := buildStreamReq("some-model", 512)
	if len(req.Tools) != 0 || req.ToolChoice != nil {
		t.Fatalf("stream probe must carry no tools/tool_choice")
	}
}
