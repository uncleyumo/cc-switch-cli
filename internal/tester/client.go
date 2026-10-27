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

// TestProvider runs the configured checks against every model configured in
// one provider, sequentially, in slot order. Results are returned in model then
// check order. A validation failure surfaces as a single fail result rather
// than crashing the batch.
func (c *Client) TestProvider(ctx context.Context, name string, m *model.Model) []Result {
	if err := model.Validate(m); err != nil {
		return []Result{{Provider: name, Check: "validate", Status: StatusFail,
			ErrorKind: ErrKindOther, Error: err.Error()}}
	}
	targets := m.Targets()
	if len(targets) == 0 {
		return []Result{{Provider: name, Check: "validate", Status: StatusFail,
			ErrorKind: ErrKindOther, Error: "no configured model id"}}
	}
	var results []Result
	for _, target := range targets {
		if c.opts.Checks&CheckStream != 0 {
			results = append(results, c.runStream(ctx, name, m, target))
		}
		if c.opts.Checks&CheckTool != 0 {
			results = append(results, c.runTool(ctx, name, m, target))
		}
	}
	return results
}

// TestAll runs every named provider in parallel bounded by Options.Concurrency.
// Within one provider every configured model is tested in order. The progress
// callback fires after each provider completes, with counts in models; safe to
// pass nil.
func (c *Client) TestAll(ctx context.Context, models map[string]*model.Model, progress func(done, total int)) []Result {
	total := 0
	for _, m := range models {
		if n := len(m.Targets()); n > 0 {
			total += n
		} else {
			total++
		}
	}
	names := make([]string, 0, len(models))
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
			n := len(models[name].Targets())
			if n == 0 {
				n = 1
			}
			mu.Lock()
			results = append(results, partial...)
			done += n
			d := done
			mu.Unlock()
			if progress != nil {
				progress(d, total)
			}
		}()
	}
	wg.Wait()
	// Deterministic order: by provider, preserving the model/check order
	// produced by TestProvider inside each provider.
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Provider < results[j].Provider
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
