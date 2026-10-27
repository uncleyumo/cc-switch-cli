package tester

import (
	"context"
	"errors"
	"net/http"
	"time"

	"cc-switch-cli/internal/model"
	"cc-switch-cli/internal/ui"
)

const (
	checkNameStream = "stream"
	checkNameTool   = "tool"
)

// runStream verifies the provider can stream a small message end-to-end for one
// configured model.
func (c *Client) runStream(parent context.Context, name string, m *model.Model, target model.ModelTarget) Result {
	ctx, cancel := context.WithTimeout(parent, c.opts.Timeout)
	defer cancel()

	modelID := target.WireID
	if modelID == "" {
		return Result{Provider: name, ModelID: modelID, ModelSlot: target.Slot,
			Check: checkNameStream, Status: StatusFail,
			ErrorKind: ErrKindOther, Error: "no model id could be resolved from this config"}
	}

	got := struct {
		start bool
		delta bool
		stop  bool
	}{}
	start := time.Now()
	status, ttft, err := c.callAndStream(ctx, m, buildStreamReq(modelID, c.opts.MaxTokens), func(ev sseEvent) (bool, error) {
		switch ev.Type {
		case "message_start":
			got.start = true
		case "content_block_delta":
			if ev.Delta != nil && ev.Delta.Text != "" {
				got.delta = true
			}
		case "message_stop":
			got.stop = true
			return true, nil
		case "error":
			if ev.Error != nil {
				return true, &ProbeError{APIType: ev.Error.Type, Message: ev.Error.Message}
			}
			return true, errors.New("provider returned error event")
		}
		return false, nil
	})
	latency := time.Since(start)
	res := Result{Provider: name, ModelID: modelID, ModelSlot: target.Slot,
		Check: checkNameStream, HTTPStatus: status, TTFT: ttft, Latency: latency}
	if err != nil {
		res.Status = StatusFail
		res.ErrorKind = errorKind(status, err)
		res.Error = ui.ScrubTokens(err.Error(), m.Token())
		return res
	}
	if !(got.start && (got.delta || got.stop)) {
		res.Status = StatusFail
		res.Error = "stream lacked message_start or any content"
		return res
	}
	res.Status = StatusOK
	return res
}

// runTool verifies the provider relays a tool_use block when the model chooses
// to call one (tool_choice is left at the API default, like Claude Code). A
// request that the endpoint rejects outright is classified by classifyToolError;
// a clean stream with no tool_use means the gateway does not surface tool calls.
func (c *Client) runTool(parent context.Context, name string, m *model.Model, target model.ModelTarget) Result {
	ctx, cancel := context.WithTimeout(parent, c.opts.Timeout)
	defer cancel()

	modelID := target.WireID
	if modelID == "" {
		return Result{Provider: name, ModelID: modelID, ModelSlot: target.Slot,
			Check: checkNameTool, Status: StatusFail,
			ErrorKind: ErrKindOther, Error: "no model id could be resolved from this config"}
	}

	sawToolUse := false
	sawStop := false
	start := time.Now()
	status, ttft, err := c.callAndStream(ctx, m, buildToolReq(modelID, c.opts.MaxTokens), func(ev sseEvent) (bool, error) {
		switch ev.Type {
		case "content_block_start":
			if ev.ContentBlock != nil && ev.ContentBlock.Type == "tool_use" {
				sawToolUse = true
			}
		case "message_delta":
			if ev.Delta != nil && ev.Delta.StopReason == "tool_use" {
				sawToolUse = true
			}
		case "message_stop":
			sawStop = true
			return true, nil
		case "error":
			if ev.Error != nil {
				return true, &ProbeError{APIType: ev.Error.Type, Message: ev.Error.Message}
			}
			return true, errors.New("provider returned error event")
		}
		return false, nil
	})
	latency := time.Since(start)
	res := Result{Provider: name, ModelID: modelID, ModelSlot: target.Slot,
		Check: checkNameTool, HTTPStatus: status, TTFT: ttft, Latency: latency}
	if err != nil {
		res.Status = classifyToolError(status)
		res.ErrorKind = errorKind(status, err)
		res.Error = ui.ScrubTokens(err.Error(), m.Token())
		return res
	}
	if !sawToolUse {
		if sawStop {
			// The request succeeded but the model answered in text instead of
			// invoking the probe tool, so we could not confirm tool_use relay.
			res.Status = StatusUnsupported
			res.Error = "provider streamed a normal response instead of calling the probe tool"
			return res
		}
		res.Status = StatusFail
		res.Error = "no tool_use content block observed"
		return res
	}
	res.Status = StatusOK
	return res
}

// classifyToolError maps a failed tool-check request to a Status. The stream
// check is the baseline availability signal, so a tool request that the
// endpoint merely rejects (a 4xx other than auth/balance/rate-limit) is treated
// as an unavailable capability (UNSUPPORTED) rather than a provider outage
// (FAIL). Authentication, balance, rate-limit and server errors stay FAIL.
func classifyToolError(status int) Status {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden: // 401 / 403
		return StatusFail
	case http.StatusPaymentRequired: // 402
		return StatusFail
	case http.StatusTooManyRequests: // 429
		return StatusFail
	case http.StatusNotImplemented: // 501: tools not implemented
		return StatusUnsupported
	}
	if status >= 400 && status < 500 {
		return StatusUnsupported
	}
	// Other 5xx, or status 0 (network error / timeout).
	return StatusFail
}
