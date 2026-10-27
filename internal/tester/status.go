package tester

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"cc-switch-cli/internal/config"
	"cc-switch-cli/internal/fsutil"
)

const statusFileName = "test-status.json"

// StatusPath returns the on-disk path of the test-status.json cache.
func StatusPath() (string, error) {
	dir, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, statusFileName), nil
}

// LoadStatus reads the test-status.json cache. A missing file returns
// (nil, nil) so callers can render "no data yet".
func LoadStatus() (*StatusFile, error) {
	path, err := StatusPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}
	var sf StatusFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}
	if sf.Results == nil {
		sf.Results = map[string]ProviderStatus{}
	}
	return &sf, nil
}

// SaveStatus atomically writes the cache to disk.
func SaveStatus(sf *StatusFile) error {
	if sf == nil {
		return errors.New("status is nil")
	}
	if sf.Results == nil {
		sf.Results = map[string]ProviderStatus{}
	}
	sf.Timestamp = time.Now().UTC().Format(time.RFC3339)
	path, err := StatusPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.AtomicWrite(path, append(data, '\n'), 0o644)
}

// Upsert folds a fresh batch of Results into the cache. Each configured model
// is collapsed to its own verdict first, then the provider takes the worst
// model verdict so `model list` shows a single, honest headline.
func (sf *StatusFile) Upsert(results []Result) {
	if sf.Results == nil {
		sf.Results = map[string]ProviderStatus{}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	byProvider := map[string][]Result{}
	for _, r := range results {
		byProvider[r.Provider] = append(byProvider[r.Provider], r)
	}
	for name, rs := range byProvider {
		// Collapse each configured model separately first, then take the worst
		// model verdict: one dead tier must not be hidden by another tier.
		byModel := map[string][]Result{}
		var modelOrder []string
		for _, r := range rs {
			if _, seen := byModel[r.ModelID]; !seen {
				modelOrder = append(modelOrder, r.ModelID)
			}
			byModel[r.ModelID] = append(byModel[r.ModelID], r)
		}
		worst := StatusOK
		for _, id := range modelOrder {
			worst = worseStatus(worst, rankChecks(byModel[id]))
		}
		if len(modelOrder) == 0 {
			worst = StatusUnsupported
		}
		latency := time.Duration(0)
		ttft := time.Duration(0)
		httpCode := 0
		errMsg := ""
		errKind := ErrKindNone
		checks := ""
		for _, r := range rs {
			if r.Latency > latency {
				latency = r.Latency
			}
			if r.TTFT > ttft {
				ttft = r.TTFT
			}
			if httpCode == 0 && r.HTTPStatus != 0 {
				httpCode = r.HTTPStatus
			}
			if errMsg == "" && r.Status != StatusOK && r.Error != "" {
				errMsg = r.Error
				if r.ModelID != "" {
					errMsg = r.ModelID + ": " + errMsg
				}
				errKind = r.ErrorKind
			}
			if checks == "" {
				checks = r.Check
			} else {
				checks += "+" + r.Check
			}
		}
		sf.Results[name] = ProviderStatus{
			Status:     worst,
			ModelCount: len(modelOrder),
			Checks:     checks,
			LatencyMS:  latency.Milliseconds(),
			TTFTMS:     ttft.Milliseconds(),
			HTTPCode:   httpCode,
			Error:      errMsg,
			ErrorKind:  errKind,
			TestedAt:   now,
		}
	}
}

// worseStatus returns the more severe of two verdicts (fail > unsupported > ok).
func worseStatus(a, b Status) Status {
	rank := func(s Status) int {
		switch s {
		case StatusFail:
			return 2
		case StatusUnsupported:
			return 1
		default:
			return 0
		}
	}
	if rank(b) > rank(a) {
		return b
	}
	return a
}

// rankChecks collapses one model's checks into a single headline verdict.
// Streaming is the source of truth for availability: a model is considered
// available as long as its stream check passed. The tool check is a secondary
// capability signal that can only DOWNGRADE an available model to
// "unsupported" (OK~) — it can never turn a stream-working model into FAIL.
// This matters because a tool probe may fail for reasons unrelated to whether
// the model is usable (a forced-call directive the gateway rejects, or a
// transient 5xx/timeout on the tool request); in both cases streaming still
// works, so the model must stay out of the FAIL bucket. A model is FAIL only
// when no check passed (i.e. streaming itself failed).
func rankChecks(rs []Result) Status {
	hasOK := false
	hasFail := false
	hasNonOK := false
	for _, r := range rs {
		switch r.Status {
		case StatusOK:
			hasOK = true
		case StatusFail:
			hasFail = true
			hasNonOK = true
		case StatusUnsupported:
			hasNonOK = true
		}
	}
	if !hasOK {
		if hasFail {
			return StatusFail
		}
		return StatusUnsupported
	}
	if hasNonOK {
		return StatusUnsupported
	}
	return StatusOK
}
