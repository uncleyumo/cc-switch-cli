// Package tester runs Anthropic Messages availability checks against provider
// configs. All test traffic is real HTTP; see cmd/model_test.go for CLI flags.
package tester

import "time"

// Status describes the outcome of a single provider check.
type Status string

const (
	StatusOK          Status = "ok"
	StatusFail        Status = "fail"
	StatusUnsupported Status = "unsupported"
)

// Check is a bitmask selecting which checks to run.
type Check uint8

const (
	CheckStream Check = 1 << iota
	CheckTool
	CheckAll = CheckStream | CheckTool
)

// AuthMode controls which auth header to use.
type AuthMode string

const (
	AuthAuto   AuthMode = "auto"
	AuthBearer AuthMode = "bearer"
	AuthAPIKey AuthMode = "x-api-key"
)

// Options controls a Client run.
type Options struct {
	Checks      Check
	Timeout     time.Duration
	Concurrency int
	Auth        AuthMode
	MaxTokens   int
}

// Result is one provider × one model × one check.
type Result struct {
	Provider   string
	ModelID    string // wire model id sent in the request
	ModelSlot  string // config slot: default/top-level/fable/opus/sonnet/haiku
	Check      string
	Status     Status
	HTTPStatus int
	TTFT       time.Duration
	Latency    time.Duration
	Error      string
	ErrorKind  ErrorKind `json:"error_kind,omitempty"`
}

// ProviderStatus is what we persist per-provider in test-status.json.
type ProviderStatus struct {
	Status     Status    `json:"status"`
	ModelCount int       `json:"model_count,omitempty"`
	Checks     string    `json:"checks,omitempty"`
	LatencyMS  int64     `json:"latency_ms,omitempty"`
	TTFTMS     int64     `json:"ttft_ms,omitempty"`
	HTTPCode   int       `json:"http_code,omitempty"`
	Error      string    `json:"error,omitempty"`
	ErrorKind  ErrorKind `json:"error_kind,omitempty"`
	TestedAt   string    `json:"tested_at"`
}

// StatusFile is the on-disk representation of the test-status.json cache.
type StatusFile struct {
	Timestamp string                    `json:"timestamp"`
	Results   map[string]ProviderStatus `json:"results"`
}
