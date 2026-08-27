package tester

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"cc-switch-cli/internal/ui"
)

// CheckLine is one per-check row inside a provider group.
type CheckLine struct {
	Check    string
	Status   Status
	TTFT     time.Duration
	Latency  time.Duration
	HTTPCode int
	Kind     ErrorKind
	Error    string
}

// Failure is one or more failed checks sharing the same verdict and error
// text, merged for display so a gateway that rejects both probes prints its
// message once instead of twice.
type Failure struct {
	Checks   []string // in original order, e.g. ["stream", "tool"]
	Status   Status   // fail or unsupported
	HTTPCode int      // first non-zero among the merged checks
	Latency  time.Duration
	Kind     ErrorKind
	Error    string
}

// ProviderGroup is the per-provider slice of a test batch, split into passing
// check lines and deduplicated failures.
type ProviderGroup struct {
	Provider string
	OKs      []CheckLine
	Failures []Failure
}

// Worst collapses the group into the headline verdict.
func (g ProviderGroup) Worst() Status {
	fail := false
	for _, f := range g.Failures {
		if f.Status == StatusFail {
			fail = true
			break
		}
	}
	switch {
	case fail:
		return StatusFail
	case len(g.Failures) > 0:
		return StatusUnsupported
	default:
		return StatusOK
	}
}

// GroupByProvider collapses a batch into per-provider display groups sorted by
// name. Failed checks that share verdict and error text are merged so their
// message is printed once; passing checks keep their own rows.
func GroupByProvider(results []Result) []ProviderGroup {
	byName := map[string][]Result{}
	names := make([]string, 0, len(results))
	for _, r := range results {
		if _, seen := byName[r.Provider]; !seen {
			names = append(names, r.Provider)
		}
		byName[r.Provider] = append(byName[r.Provider], r)
	}
	sort.Strings(names)

	groups := make([]ProviderGroup, 0, len(names))
	for _, name := range names {
		g := ProviderGroup{Provider: name}
		merged := map[string]int{} // status|error -> index into g.Failures
		for _, r := range byName[name] {
			if r.Status == StatusOK {
				g.OKs = append(g.OKs, CheckLine{
					Check: r.Check, Status: r.Status,
					TTFT: r.TTFT, Latency: r.Latency, HTTPCode: r.HTTPStatus,
				})
				continue
			}
			key := string(r.Status) + "|" + r.Error
			if idx, seen := merged[key]; seen {
				g.Failures[idx].Checks = append(g.Failures[idx].Checks, r.Check)
				if g.Failures[idx].HTTPCode == 0 && r.HTTPStatus != 0 {
					g.Failures[idx].HTTPCode = r.HTTPStatus
				}
				if r.Latency > g.Failures[idx].Latency {
					g.Failures[idx].Latency = r.Latency
				}
				continue
			}
			merged[key] = len(g.Failures)
			g.Failures = append(g.Failures, Failure{
				Checks:   []string{r.Check},
				Status:   r.Status,
				HTTPCode: r.HTTPStatus,
				Latency:  r.Latency,
				Kind:     r.ErrorKind,
				Error:    r.Error,
			})
		}
		groups = append(groups, g)
	}
	return groups
}

// providerTally folds the batch into provider counts per headline verdict.
func providerTally(results []Result) (providers, ok, fail, unsupported int) {
	byProvider := map[string][]Result{}
	names := make([]string, 0, len(results))
	for _, r := range results {
		if _, seen := byProvider[r.Provider]; !seen {
			names = append(names, r.Provider)
		}
		byProvider[r.Provider] = append(byProvider[r.Provider], r)
	}
	sort.Strings(names)
	for _, name := range names {
		switch statusRank(byProvider[name]) {
		case StatusOK:
			ok++
		case StatusFail:
			fail++
		default:
			unsupported++
		}
	}
	return len(names), ok, fail, unsupported
}

// SummarizeStyled is Summarize with the counts tinted by verdict (ok green,
// fail red, unsupported amber), so the numbers read at a glance on both the
// CLI and in the TUI result view.
func SummarizeStyled(results []Result) string {
	if len(results) == 0 {
		return "no results"
	}
	providers, ok, fail, unsupported := providerTally(results)
	parts := make([]string, 0, 3)
	if ok > 0 {
		parts = append(parts, ui.Success(fmt.Sprintf("%d ok", ok)))
	}
	if fail > 0 {
		parts = append(parts, ui.Fail(fmt.Sprintf("%d fail", fail)))
	}
	if unsupported > 0 {
		parts = append(parts, ui.Warn(fmt.Sprintf("%d unsupported", unsupported)))
	}
	return fmt.Sprintf("tested %d providers: %s", providers, strings.Join(parts, ", "))
}
