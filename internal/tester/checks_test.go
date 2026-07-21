package tester

import (
	"net/http"
	"testing"
)

func TestClassifyToolError(t *testing.T) {
	cases := []struct {
		status int
		want   Status
	}{
		{http.StatusUnauthorized, StatusFail},               // 401 auth
		{http.StatusForbidden, StatusFail},                  // 403 auth
		{http.StatusPaymentRequired, StatusFail},            // 402 balance
		{http.StatusTooManyRequests, StatusFail},            // 429 rate limit
		{http.StatusBadRequest, StatusUnsupported},          // 400 tool request rejected
		{http.StatusNotFound, StatusUnsupported},            // 404
		{http.StatusUnprocessableEntity, StatusUnsupported}, // 422 (litellm translation)
		{http.StatusNotImplemented, StatusUnsupported},      // 501 tools not implemented
		{http.StatusServiceUnavailable, StatusFail},         // 503 outage
		{http.StatusInternalServerError, StatusFail},        // 500
		{0, StatusFail}, // network error / timeout
	}
	for _, c := range cases {
		if got := classifyToolError(c.status); got != c.want {
			t.Errorf("classifyToolError(%d) = %q, want %q", c.status, got, c.want)
		}
	}
}

// TestUpsertProviderRollup verifies that streaming is the source of truth for
// the headline verdict: a stream-OK provider never lands in FAIL, even when the
// tool check is unsupported or transiently errored, while a stream-FAIL provider
// stays FAIL.
func TestUpsertProviderRollup(t *testing.T) {
	sf := &StatusFile{Results: map[string]ProviderStatus{}}
	sf.Upsert([]Result{
		// stream OK + tool unsupported -> usable with caveat
		{Provider: "partial", Check: "stream", Status: StatusOK, HTTPStatus: 200},
		{Provider: "partial", Check: "tool", Status: StatusUnsupported, HTTPStatus: 422},
		// stream OK + tool transiently errored (e.g. 520/timeout) -> still usable,
		// NOT FAIL: streaming proved the provider is up.
		{Provider: "flakytool", Check: "stream", Status: StatusOK, HTTPStatus: 200},
		{Provider: "flakytool", Check: "tool", Status: StatusFail, HTTPStatus: 520},
		// stream FAIL -> FAIL regardless of tool
		{Provider: "broken", Check: "stream", Status: StatusFail, HTTPStatus: 400},
		{Provider: "broken", Check: "tool", Status: StatusUnsupported, HTTPStatus: 400},
		// both FAIL (no check passed) -> FAIL
		{Provider: "down", Check: "stream", Status: StatusFail, HTTPStatus: 503},
		{Provider: "down", Check: "tool", Status: StatusFail, HTTPStatus: 503},
		// both OK -> OK
		{Provider: "healthy", Check: "stream", Status: StatusOK, HTTPStatus: 200},
		{Provider: "healthy", Check: "tool", Status: StatusOK, HTTPStatus: 200},
	})
	cases := map[string]Status{
		"partial":   StatusUnsupported,
		"flakytool": StatusUnsupported,
		"broken":    StatusFail,
		"down":      StatusFail,
		"healthy":   StatusOK,
	}
	for name, want := range cases {
		if got := sf.Results[name].Status; got != want {
			t.Errorf("provider %q = %q, want %q", name, got, want)
		}
	}
}
