package tester

import (
	"context"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	"cc-switch-cli/internal/model"
)

// Client runs availability checks. Zero value is not valid; use NewClient.
type Client struct {
	opts Options
}

// NewClient returns a Client with normalized options.
func NewClient(opts Options) *Client {
	if opts.Checks == 0 {
		opts.Checks = CheckAll
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = 4
	}
	if opts.Concurrency > 16 {
		opts.Concurrency = 16
	}
	if opts.Auth == "" {
		opts.Auth = AuthAuto
	}
	if opts.MaxTokens <= 0 {
		// Thinking models spend part of the budget on reasoning before emitting
		// text or a tool_use block; keep enough headroom so the tool check is not
		// starved into a false negative.
		opts.MaxTokens = 512
	}
	return &Client{opts: opts}
}

// TestProvider runs the configured checks against one provider sequentially.
// Results are returned in check order. A validation failure surfaces as a
// single fail result rather than crashing the batch.
func (c *Client) TestProvider(ctx context.Context, name string, m *model.Model) []Result {
	if err := model.Validate(m); err != nil {
		return []Result{{Provider: name, Check: "validate", Status: StatusFail, Error: err.Error()}}
	}
	var results []Result
	if c.opts.Checks&CheckStream != 0 {
		results = append(results, c.runStream(ctx, name, m))
	}
	if c.opts.Checks&CheckTool != 0 {
		results = append(results, c.runTool(ctx, name, m))
	}
	return results
}

// TestAll runs every named provider in parallel bounded by Options.Concurrency.
// The progress callback fires after each provider completes; safe to pass nil.
func (c *Client) TestAll(ctx context.Context, models map[string]*model.Model, progress func(done, total int)) []Result {
	total := len(models)
	names := make([]string, 0, total)
	for n := range models {
		names = append(names, n)
	}
	sort.Strings(names)

	sem := make(chan struct{}, c.opts.Concurrency)
	var (
		mu      sync.Mutex
		results []Result
		done    int
		wg      sync.WaitGroup
	)
	for _, name := range names {
		name := name
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			partial := c.TestProvider(ctx, name, models[name])
			mu.Lock()
			results = append(results, partial...)
			done++
			d := done
			mu.Unlock()
			if progress != nil {
				progress(d, total)
			}
		}()
	}
	wg.Wait()
	// Deterministic order: by provider, then by check within a provider.
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Provider != results[j].Provider {
			return results[i].Provider < results[j].Provider
		}
		return results[i].Check < results[j].Check
	})
	return results
}

// readAll is a tiny helper to bound how much we read from an error body.
func readErrorBody(r io.Reader) string {
	buf := make([]byte, 4096)
	n, _ := io.ReadFull(io.LimitReader(r, int64(len(buf))), buf)
	if n <= 0 {
		return ""
	}
	return string(buf[:n])
}

// fmtErr prefixes a message with the check so it's clear what failed.
func fmtErr(check, msg string) string {
	return fmt.Sprintf("%s: %s", check, msg)
}
