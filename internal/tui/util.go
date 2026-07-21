package tui

import "encoding/json"

// jsonQuote encodes s as a JSON string RawMessage.
func jsonQuote(s string) (json.RawMessage, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}
