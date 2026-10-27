package tester

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"cc-switch-cli/internal/ui"
)

// CheckLine is one per-check row inside a model group.
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

// ModelGroup is the per-model slice of a test batch. A provider with several
// configured model ids gets one ModelGroup per id; each is rendered with its
// own verdict so one dead tier does not hide the others.
type ModelGroup struct {
	Slot     string // config slot: default/top-level/fable/opus/sonnet/haiku
	ModelID  string // wire model id
	OKs      []CheckLine
	Failures []Failure
}

// Worst collapses one model into its headline verdict.
func (g ModelGroup) Worst() Status {
	for _, f := range g.Failures {
		if f.Status == StatusFail {
			return StatusFail
		}
	}
	if len(g.Failures) > 0 {
		return StatusUnsupported
	}
	return StatusOK
}

// Label renders the model group heading, e.g. "sonnet · claude-sonnet-5-5".
func (g ModelGroup) Label() string {
	if g.ModelID == "" {
		return ""
	}
	if g.Slot == "" {
		return g.ModelID
	}
	return g.Slot + " · " + g.ModelID
}

// ProviderGroup is one provider's results, split per configured model.
type ProviderGroup struct {
	Provider string
	Models   []ModelGroup
}

// Worst collapses the provider into the worst verdict among its models: a
// provider is only fully OK when every configured model is OK.
func (g ProviderGroup) Worst() Status {
	worst := StatusOK
	for _, m := range g.Models {
		switch m.Worst() {
		case StatusFail:
			return StatusFail
		case StatusUnsupported:
			worst = StatusUnsupported
		}
	}
	return worst
}

// GroupByProvider collapses a batch into per-provider, per-model display
// groups sorted by provider name. Within a provider the configured slot order
// is preserved. Failed checks that share verdict and error text are merged so
// their message is printed once; passing checks keep their own rows.
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
		var modelOrder []string
		byModel := map[string][]Result{}
		slots := map[string]string{}
		for _, r := range byName[name] {
			if _, seen := byModel[r.ModelID]; !seen {
				modelOrder = append(modelOrder, r.ModelID)
				slots[r.ModelID] = r.ModelSlot
			}
			byModel[r.ModelID] = append(byModel[r.ModelID], r)
		}
		for _, id := range modelOrder {
			g.Models = append(g.Models, buildModelGroup(slots[id], id, byModel[id]))
		}
		groups = append(groups, g)
	}
	return groups
}

// buildModelGroup splits one model's results into passing lines and merged
// failures, preserving result order.
func buildModelGroup(slot, id string, results []Result) ModelGroup {
	g := ModelGroup{Slot: slot, ModelID: id}
	merged := map[string]int{} // status|error -> index into g.Failures
	for _, r := range results {
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
	return g
}

// modelTally folds the batch into model counts per headline verdict, plus the
// number of providers those models belong to.
func modelTally(results []Result) (providers, models, ok, fail, unsupported int) {
	groups := GroupByProvider(results)
	for _, g := range groups {
		for _, m := range g.Models {
			models++
			switch m.Worst() {
			case StatusOK:
				ok++
			case StatusFail:
				fail++
			default:
				unsupported++
			}
		}
	}
	return len(groups), models, ok, fail, unsupported
}

// SummarizeStyled is Summarize with the counts tinted by verdict (ok green,
// fail red, unsupported amber), so the numbers read at a glance on both the
// CLI and in the TUI result view.
func SummarizeStyled(results []Result) string {
	if len(results) == 0 {
		return "no results"
	}
	providers, models, ok, fail, unsupported := modelTally(results)
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
	return summarizeLine(providers, models) + ": " + strings.Join(parts, ", ")
}

// summarizeLine names the batch as providers, and adds the model count when a
// provider carries more than one configured model.
func summarizeLine(providers, models int) string {
	if models == providers {
		return fmt.Sprintf("tested %d providers", providers)
	}
	return fmt.Sprintf("tested %d providers, %d models", providers, models)
}
