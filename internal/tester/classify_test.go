package tester

import (
	"errors"
	"testing"
)

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name   string
		status int
		apiTyp string
		msg    string
		want   ErrorKind
	}{
		// Anthropic error.type passthrough.
		{"auth type", 0, "authentication_error", "", ErrKindAuth},
		{"permission type beats status", 500, "permission_error", "http 500: nope", ErrKindAuth},
		{"balance type", 0, "insufficient_balance", "", ErrKindAuth},
		{"rate limit type", 0, "rate_limit_error", "", ErrKindRateLimit},
		{"invalid request type", 400, "invalid_request_error", "", ErrKindBadRequest},
		{"not found type", 0, "not_found_error", "", ErrKindNotFound},
		{"overloaded type on 2xx stream error", 200, "overloaded_error", "Overloaded", ErrKindOverloaded},
		{"api error type", 0, "api_error", "", ErrKindServerError},
		// Unrecognized gateway types fall through to the status code.
		{"unknown type falls to status", 429, "InvalidParameter", "", ErrKindRateLimit},
		{"unknown type falls to message", 200, "GatewayBuzz", "dial tcp: refused", ErrKindNetwork},

		// Message markers.
		{"empty stream marker", 200, "", "no events received", ErrKindEmptyResp},
		{"client timeout", 0, "", `Get "https://x": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`, ErrKindTimeout},
		{"context deadline", 0, "", "context deadline exceeded", ErrKindTimeout},
		{"dial tcp", 0, "", `Get "https://x": dial tcp 1.2.3.4:443: connectex: connection refused`, ErrKindNetwork},
		{"dns", 0, "", `Get "https://x": no such host`, ErrKindNetwork},
		{"tls", 0, "", "tls: handshake failure", ErrKindNetwork},
		{"i/o timeout is network not timeout", 0, "", "read tcp: i/o timeout", ErrKindNetwork},
		{"markers are case insensitive", 0, "", "DIAL TCP failed", ErrKindNetwork},

		// HTTP status buckets.
		{"401", 401, "", "http 401: bad key", ErrKindAuth},
		{"402", 402, "", "http 402: out of credit", ErrKindAuth},
		{"403", 403, "", "http 403: forbidden", ErrKindAuth},
		{"404", 404, "", "http 404: not found", ErrKindNotFound},
		{"429", 429, "", "http 429: slow down", ErrKindRateLimit},
		{"501", 501, "", "http 501: no tools", ErrKindUnsupported},
		{"504 vs 500", 504, "", "http 504: gateway", ErrKindTimeout},
		{"500", 500, "", "http 500: boom", ErrKindServerError},
		{"418 is bad request", 418, "", "http 418: teapot", ErrKindBadRequest},
		// Status code wins when the message contains transport-like noise
		// but there was a real response.
		{"status beats odd message", 500, "", "http 500: upstream connection reset", ErrKindServerError},

		// Fallbacks.
		{"ok and quiet", 200, "", "", ErrKindNone},
		{"ok with message", 200, "", "stream lacked message_start or any content", ErrKindOther},
		{"nothing at all", 0, "", "", ErrKindNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyError(tt.status, tt.apiTyp, tt.msg); got != tt.want {
				t.Errorf("ClassifyError(%d, %q, %q) = %q, want %q", tt.status, tt.apiTyp, tt.msg, got, tt.want)
			}
		})
	}
}

func TestErrorKindHuman(t *testing.T) {
	kinds := []ErrorKind{
		ErrKindNone, ErrKindAuth, ErrKindRateLimit, ErrKindBadRequest,
		ErrKindNotFound, ErrKindServerError, ErrKindOverloaded,
		ErrKindTimeout, ErrKindNetwork, ErrKindEmptyResp,
		ErrKindUnsupported, ErrKindOther,
	}
	for _, k := range kinds {
		if k == ErrKindNone {
			if got := k.Human(); got != "" {
				t.Errorf("ErrorKind(\"\").Human() = %q, want empty", got)
			}
			continue
		}
		if got := k.Human(); got == "" {
			t.Errorf("ErrorKind(%q).Human() = \"\", want non-empty", k)
		}
	}
}

func TestProbeErrorClassification(t *testing.T) {
	pe := &ProbeError{HTTPStatus: 401, APIType: "authentication_error", Message: "http 401: invalid x-api-key"}
	if got := errorKind(0, pe); got != ErrKindAuth {
		t.Errorf("errorKind via ProbeError = %q, want auth", got)
	}
	// Structured fields win over the wrapper's status argument.
	if got := errorKind(500, pe); got != ErrKindAuth {
		t.Errorf("errorKind should prefer ProbeError.HTTPStatus, got %q", got)
	}
	// Plain errors fall back to the recorded status + prose matching.
	err := errors.New(`Get "x": dial tcp refused`)
	if got := errorKind(0, err); got != ErrKindNetwork {
		t.Errorf("errorKind plain = %q, want network", got)
	}
	if got := errorKind(0, nil); got != ErrKindNone {
		t.Errorf("errorKind(nil) = %q, want none", got)
	}
}
