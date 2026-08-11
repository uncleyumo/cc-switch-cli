package transfer

import (
	"encoding/json"
	"fmt"

	"cc-switch-cli/internal/mcp"
	"cc-switch-cli/internal/model"
)

// BuildPayload reads the selected model files and user-scope MCP servers.
// Models are stored as their exact on-disk bytes, MCP servers as their
// canonical encoding. Unknown or malformed entries produce errors naming
// the offender.
func BuildPayload(modelNames, mcpNames []string) (*Payload, error) {
	p := &Payload{}
	if len(modelNames) > 0 {
		p.Models = make(map[string]json.RawMessage, len(modelNames))
		for _, name := range modelNames {
			data, err := model.ReadRaw(name)
			if err != nil {
				return nil, err
			}
			if !json.Valid(data) {
				return nil, fmt.Errorf("model %q is not valid JSON", name)
			}
			if _, err := model.Parse(data); err != nil {
				return nil, fmt.Errorf("model %q is malformed: %w", name, err)
			}
			p.Models[name] = data
		}
	}
	if len(mcpNames) > 0 {
		p.MCPs = make(map[string]json.RawMessage, len(mcpNames))
		for _, name := range mcpNames {
			srv, err := mcp.Get(mcp.ScopeUser, "", name)
			if err != nil {
				return nil, err
			}
			data, err := json.Marshal(srv)
			if err != nil {
				return nil, fmt.Errorf("failed to encode mcp server %q: %w", name, err)
			}
			p.MCPs[name] = data
		}
	}
	return p, nil
}
