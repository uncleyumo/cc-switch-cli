package tester

import (
	"errors"
	"net/http"
	"strings"
)

// ErrorKind is a coarse, machine-stable classification of why a check failed.
// It is persisted alongside Status in test-status.json so both the CLI table
// and the TUI can show a short reason instead of leaving the user guessing.
type ErrorKind string

const (
	ErrKindNone        ErrorKind = ""               // no failure (never persisted; omitempty)
	ErrKindAuth        ErrorKind = "auth"           // 401/402/403, authentication_error, balance issues
	ErrKindRateLimit   ErrorKind = "rate_limit"     // 429, rate_limit_error
	ErrKindBadRequest  ErrorKind = "bad_request"    // 400/422 and other unmapped 4xx, invalid_request_error
	ErrKindNotFound    ErrorKind = "not_found"      // 404, not_found_error (bad model id / base URL)
	ErrKindServerError ErrorKind = "server_error"   // 5xx (except 504), api_error
	ErrKindOverloaded  ErrorKind = "overloaded"     // overloaded_error
	ErrKindTimeout     ErrorKind = "timeout"        // Client.Timeout, context deadline exceeded, 504
	ErrKindNetwork     ErrorKind = "network"        // dial tcp failures, DNS, TLS, connection reset
	ErrKindEmptyResp   ErrorKind = "empty_response" // stream opened but produced nothing
	ErrKindUnsupported ErrorKind = "unsupported"    // capability gaps surfaced as a kind (501-style)
	ErrKindOther       ErrorKind = "other"          // in-stream provider error without a type, misc
)

// ClassifyError maps the raw evidence collected by a probe into an ErrorKind.
// The inputs may be partially filled (in-stream SSE errors carry an apiType but
// no HTTP status; transport errors carry neither). Precedence:
//
//  1. Anthropic error.type passthrough (real semantics beat gateway HTTP codes)
//  2. explicit message markers ("no events received")
//  3. transport / timeout patterns in the message
//  4. HTTP status buckets
//  5. fallback: empty message -> none, otherwise other
func ClassifyError(httpStatus int, apiType, msg string) ErrorKind {
	if k := classifyAPIType(apiType); k != "" {
		return k
	}
	switch msg {
	case "no events received":
		return ErrKindEmptyResp
	}
	// A real HTTP error response means the endpoint was reached, so the status
	// code is stronger evidence than any transport-like noise relayed inside
	// its body ("http 500: upstream connection reset").
	if httpStatus >= 400 {
		return classifyHTTPStatus(httpStatus, msg == "")
	}
	msgLower := strings.ToLower(msg)
	if strings.Contains(msgLower, "client.timeout exceeded") ||
		strings.Contains(msgLower, "context deadline exceeded") {
		return ErrKindTimeout
	}
	for _, marker := range []string{
		"dial tcp", "no such host", "connection refused", "connection reset",
		"network is unreachable", "no route to host", "broken pipe",
		"proxyconnect", "tls:", "x509",
	} {
		if strings.Contains(msgLower, marker) {
			return ErrKindNetwork
		}
	}
	if strings.Contains(msgLower, "i/o timeout") {
		return ErrKindNetwork
	}
	return classifyHTTPStatus(httpStatus, msg == "")
}

// classifyAPIType translates Anthropic error.type values. Unrecognized types
// (gateway-specific names like Aliyun's InvalidParameter) return "" so the
// HTTP status code decides instead of an invented mapping.
func classifyAPIType(apiType string) ErrorKind {
	switch strings.ToLower(strings.TrimSpace(apiType)) {
	case "authentication_error", "permission_error", "invalid_api_key",
		"insufficient_balance", "insufficient_credits", "billing_error":
		return ErrKindAuth
	case "rate_limit_error":
		return ErrKindRateLimit
	case "invalid_request_error", "request_too_large", "invalid_parameter":
		return ErrKindBadRequest
	case "not_found_error":
		return ErrKindNotFound
	case "overloaded_error":
		return ErrKindOverloaded
	case "api_error":
		return ErrKindServerError
	}
	return ""
}

// classifyHTTPStatus buckets an HTTP status code. allQuiet is true when there
// is no message at all, which distinguishes "nothing went wrong" from "unknown".
func classifyHTTPStatus(status int, allQuiet bool) ErrorKind {
	switch {
	case status >= 400 && status < 500:
		switch status {
		case http.StatusUnauthorized, http.StatusPaymentRequired, http.StatusForbidden:
			return ErrKindAuth
		case http.StatusNotFound:
			return ErrKindNotFound
		case http.StatusTooManyRequests:
			return ErrKindRateLimit
		}
		return ErrKindBadRequest
	case status == http.StatusGatewayTimeout:
		return ErrKindTimeout
	case status >= 500 && status < 600:
		if status == http.StatusNotImplemented {
			return ErrKindUnsupported
		}
		return ErrKindServerError
	case status < 400:
		// A 2xx/3xx with an error message means the failure happened after the
		// headers (e.g. in-stream): without further evidence it stays generic.
		if allQuiet {
			return ErrKindNone
		}
		return ErrKindOther
	default:
		if allQuiet {
			return ErrKindNone
		}
		return ErrKindOther
	}
}

// Human renders the kind for display. Zero value means "no reason known".
func (k ErrorKind) Human() string {
	switch k {
	case ErrKindAuth:
		return "auth error"
	case ErrKindRateLimit:
		return "rate limited"
	case ErrKindBadRequest:
		return "bad request"
	case ErrKindNotFound:
		return "not found"
	case ErrKindServerError:
		return "server error"
	case ErrKindOverloaded:
		return "overloaded"
	case ErrKindTimeout:
		return "timed out"
	case ErrKindNetwork:
		return "network error"
	case ErrKindEmptyResp:
		return "empty response"
	case ErrKindUnsupported:
		return "unsupported"
	case ErrKindOther:
		return "error"
	default:
		return ""
	}
}

// ProbeError carries structured failure info through the check pipeline so
// callers can classify from fields instead of re-parsing prose.
type ProbeError struct {
	HTTPStatus int    // 0 when no response was received
	APIType    string // Anthropic error.type, e.g. "authentication_error"
	Message    string // human-readable detail (classified before token scrubbing)
}

func (e *ProbeError) Error() string { return e.Message }

// errorKind folds any probe error (structured or plain) plus the recorded HTTP
// status into an ErrorKind.
func errorKind(httpStatus int, err error) ErrorKind {
	var pe *ProbeError
	if errors.As(err, &pe) {
		return ClassifyError(pe.HTTPStatus, pe.APIType, pe.Message)
	}
	if err == nil {
		return ErrKindNone
	}
	return ClassifyError(httpStatus, "", err.Error())
}
