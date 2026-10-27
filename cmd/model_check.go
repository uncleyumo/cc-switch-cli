package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"cc-switch-cli/internal/manager"
	"cc-switch-cli/internal/model"
	"cc-switch-cli/internal/tester"
	"cc-switch-cli/internal/ui"
)

var (
	modelTestStream      bool
	modelTestTool        bool
	modelTestTimeout     time.Duration
	modelTestConcurrency int
	modelTestAuth        string
	modelTestStrict      bool
)

// errModelTestFailed is a sentinel returned from RunE so main maps it to exit
// code 1; SilenceErrors suppresses the redundant echo after the table and the
// detail blocks have already explained every failure.
var errModelTestFailed = errors.New("one or more checks failed")

// shouldExitNonZero decides the command's exit code. Any FAIL verdict fails
// the run; UNSUPPORTED only counts when --strict was passed, since it usually
// means a missing capability rather than an outage.
func shouldExitNonZero(results []tester.Result, strict bool) bool {
	for _, r := range results {
		switch r.Status {
		case tester.StatusFail:
			return true
		case tester.StatusUnsupported:
			if strict {
				return true
			}
		}
	}
	return false
}

var modelTestCmd = &cobra.Command{
	Use:   "test [model...]",
	Short: "Run availability checks against one or more models",
	Long: "Runs Anthropic streaming and/or tool-call checks against every configured model of each named provider. Without arguments, tests every model of every provider.\n\n" +
		"Failed checks are classified (auth, rate limit, timeout, network, ...) and the full error is printed below each model.\n\n" +
		"Exit status: 1 when any check fails; with --strict, an UNSUPPORTED verdict also fails.\n\n" +
		"Warning: this issues real API calls against each provider's endpoint and may incur a tiny cost.",
	RunE: func(cmd *cobra.Command, args []string) error {
		checks := tester.Check(0)
		if modelTestStream {
			checks |= tester.CheckStream
		}
		if modelTestTool {
			checks |= tester.CheckTool
		}
		if checks == 0 {
			checks = tester.CheckAll
		}

		var names []string
		if len(args) == 0 {
			listed, err := manager.ListModels()
			if err != nil {
				return err
			}
			names = listed
		} else {
			names = args
		}
		if len(names) == 0 {
			return errors.New("no models to test")
		}

		models := make(map[string]*model.Model, len(names))
		for _, n := range names {
			m, err := model.LoadByName(n)
			if err != nil {
				fmt.Fprintln(os.Stderr, ui.Fail("skip "+n+": "+err.Error()))
				continue
			}
			models[n] = m
		}
		if len(models) == 0 {
			return errors.New("no valid models could be loaded")
		}

		auth, err := parseAuthMode(modelTestAuth)
		if err != nil {
			return err
		}
		client := tester.NewClient(tester.Options{
			Checks:      checks,
			Timeout:     modelTestTimeout,
			Concurrency: modelTestConcurrency,
			Auth:        auth,
		})

		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()
		var progress func(done, total int)
		if term.IsTerminal(int(os.Stderr.Fd())) {
			progress = func(done, total int) {
				fmt.Fprintf(os.Stderr, "\rtesting models %d/%d...", done, total)
			}
		}
		results := client.TestAll(ctx, models, progress)
		if progress != nil {
			fmt.Fprintln(os.Stderr)
		}
		tester.PrintResults(os.Stdout, results)

		status, _ := tester.LoadStatus()
		if status == nil {
			status = &tester.StatusFile{Results: map[string]tester.ProviderStatus{}}
		}
		status.Upsert(results)
		if err := tester.SaveStatus(status); err != nil {
			fmt.Fprintln(os.Stderr, ui.Warn("warning: failed to persist test status: "+err.Error()))
		}
		if shouldExitNonZero(results, modelTestStrict) {
			cmd.SilenceErrors = true
			return errModelTestFailed
		}
		return nil
	},
}

func parseAuthMode(s string) (tester.AuthMode, error) {
	switch s {
	case "", "auto":
		return tester.AuthAuto, nil
	case "bearer":
		return tester.AuthBearer, nil
	case "x-api-key", "api-key":
		return tester.AuthAPIKey, nil
	default:
		return "", fmt.Errorf("unknown --auth mode %q (want auto, bearer or x-api-key)", s)
	}
}

func init() {
	modelTestCmd.Flags().BoolVar(&modelTestStream, "stream", false, "Only run the streaming check")
	modelTestCmd.Flags().BoolVar(&modelTestTool, "tool", false, "Only run the tool-call check")
	modelTestCmd.Flags().DurationVar(&modelTestTimeout, "timeout", 30*time.Second, "Per-provider timeout")
	modelTestCmd.Flags().IntVar(&modelTestConcurrency, "concurrency", 4, "Maximum concurrent providers under test")
	modelTestCmd.Flags().StringVar(&modelTestAuth, "auth", "auto", "Auth header mode: auto, bearer or x-api-key")
	modelTestCmd.Flags().BoolVar(&modelTestStrict, "strict", false, "Also fail on UNSUPPORTED verdicts (for CI)")
}
