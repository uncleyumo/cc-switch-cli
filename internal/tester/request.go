package tester

// messagesRequest is the /v1/messages request body cc-switch-cli sends.
// Only the fields we use are declared; unknown fields on responses go through
// the sse parser separately.
type messagesRequest struct {
	Model      string      `json:"model"`
	MaxTokens  int         `json:"max_tokens"`
	Stream     bool        `json:"stream"`
	Messages   []message   `json:"messages"`
	Tools      []tool      `json:"tools,omitempty"`
	ToolChoice *toolChoice `json:"tool_choice,omitempty"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema inputSchema `json:"input_schema"`
}

type inputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]propType `json:"properties"`
	Required   []string            `json:"required,omitempty"`
}

type propType struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

type toolChoice struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

// buildStreamReq creates a minimal streaming request used for the stream check.
func buildStreamReq(modelID string, maxTokens int) messagesRequest {
	return messagesRequest{
		Model:     modelID,
		MaxTokens: maxTokens,
		Stream:    true,
		Messages: []message{{
			Role:    "user",
			Content: "Reply with the single word: pong.",
		}},
	}
}

// buildToolReq creates a streaming request that asks the model to call a tool,
// mirroring how Claude Code itself issues tool calls: the tool list is supplied
// but tool_choice is LEFT UNSET (the API default, "auto"), so the model decides
// to call it rather than being forced.
//
// We deliberately do NOT send tool_choice:"any". That forced-call directive is
// unimplemented by several Anthropic-compatible gateways (e.g. Aliyun maas
// returns 400, litellm-backed OpenAI proxies return 422), even though those same
// gateways relay tool_use blocks perfectly under the default "auto" mode that
// Claude Code uses. Forcing it therefore produced false "tool failed" verdicts
// for providers whose tool calling actually works. A firm instruction in the
// prompt reliably elicits the call without the incompatible directive.
func buildToolReq(modelID string, maxTokens int) messagesRequest {
	return messagesRequest{
		Model:     modelID,
		MaxTokens: maxTokens,
		Stream:    true,
		Messages: []message{{
			Role:    "user",
			Content: "You must call the ccs_probe tool now with status set to \"ok\". Do not write any text, only invoke the tool.",
		}},
		Tools: []tool{{
			Name:        "ccs_probe",
			Description: "cc-switch-cli availability probe. Invoke this exactly once.",
			InputSchema: inputSchema{
				Type: "object",
				Properties: map[string]propType{
					"status": {Type: "string", Description: "Always send \"ok\"."},
				},
				Required: []string{"status"},
			},
		}},
	}
}
