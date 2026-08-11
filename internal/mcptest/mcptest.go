// Package mcptest is a minimal read-only MCP client used to probe server
// availability (initialize handshake) and list tools. It speaks stdio and
// Streamable HTTP, with a legacy HTTP+SSE fallback, using only the standard
// library plus the internal/mcp domain types.
package mcptest

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"cc-switch-cli/internal/mcp"
)

// Status is the headline verdict of a probe.
type Status string

const (
	StatusOK          Status = "ok"
	StatusFail        Status = "fail"
	StatusUnsupported Status = "unsupported"
)

// ToolInfo is one entry from tools/list.
type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Result is the outcome of probing one server.
type Result struct {
	Name      string     `json:"name"`
	Scope     string     `json:"scope"`
	Status    Status     `json:"status"`
	Transport string     `json:"transport"`
	LatencyMS int64      `json:"latency_ms"`
	ToolCount int        `json:"tool_count"` // -1 when tools/list failed or was skipped
	Tools     []ToolInfo `json:"tools,omitempty"`
	Error     string     `json:"error,omitempty"`
	TestedAt  string     `json:"tested_at"`
}

// Options controls probe behaviour.
type Options struct {
	Timeout     time.Duration // per-server total; <=0 -> 30s
	Concurrency int           // <=0 -> 4
	ProjectDir  string        // sets CLAUDE_PROJECT_DIR for stdio servers
	WithTools   bool          // fetch tools/list (probe availability only when false)
}

func normalizeOptions(opts Options) Options {
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = 4
	}
	if opts.Concurrency > 16 {
		opts.Concurrency = 16
	}
	return opts
}

// Probe connects and runs the handshake for one server. When opts.WithTools is
// set it also lists tools. An initialize success with tools/list failure is
// still StatusOK with ToolCount -1 and a warning in Error.
func Probe(ctx context.Context, e mcp.Entry, opts Options) Result {
	opts = normalizeOptions(opts)
	start := time.Now()
	r := Result{
		Name:      e.Name,
		Scope:     string(e.Scope),
		ToolCount: -1,
		TestedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	if e.Server == nil {
		r.Status = StatusFail
		r.Error = "empty server entry"
		r.LatencyMS = time.Since(start).Milliseconds()
		return r
	}
	r.Transport = string(e.Server.Transport())

	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	switch e.Server.Transport() {
	case mcp.TransportWS:
		r.Status = StatusUnsupported
		r.Error = "ws transport is not supported by the probe"
	case mcp.TransportInvalid:
		r.Status = StatusFail
		if err := e.Server.Validate(); err != nil {
			r.Error = err.Error()
		} else {
			r.Error = "server entry is not a valid MCP configuration"
		}
	case mcp.TransportStdio:
		probeStdio(ctx, e, opts, &r)
	case mcp.TransportHTTP, mcp.TransportSSE:
		probeHTTP(ctx, e, opts, &r)
	default:
		r.Status = StatusFail
		r.Error = fmt.Sprintf("unknown transport %q", e.Server.Transport())
	}
	r.LatencyMS = time.Since(start).Milliseconds()
	return r
}

// ProbeAll runs probes with a worker pool. Results are returned in input order.
// progress is called after each completion (safe to pass nil).
func ProbeAll(ctx context.Context, entries []mcp.Entry, opts Options, progress func(done, total int)) []Result {
	opts = normalizeOptions(opts)
	total := len(entries)
	results := make([]Result, total)
	if total == 0 {
		return results
	}

	sem := make(chan struct{}, opts.Concurrency)
	var (
		mu   sync.Mutex
		done int
		wg   sync.WaitGroup
	)
	for i, e := range entries {
		i, e := i, e
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[i] = Result{
					Name:      e.Name,
					Scope:     string(e.Scope),
					Status:    StatusFail,
					Error:     ctx.Err().Error(),
					ToolCount: -1,
					TestedAt:  time.Now().UTC().Format(time.RFC3339),
				}
				return
			}
			defer func() { <-sem }()
			results[i] = Probe(ctx, e, opts)
			mu.Lock()
			done++
			d := done
			mu.Unlock()
			if progress != nil {
				progress(d, total)
			}
		}()
	}
	wg.Wait()
	return results
}

// CacheKey builds the status-cache key "scope/name".
func CacheKey(scope mcp.Scope, name string) string {
	return string(scope) + "/" + name
}

// sortedNames is a tiny helper used by tests.
func sortedNames(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
