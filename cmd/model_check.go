package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"

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
)

var modelTestCmd = &cobra.Command{
	Use:   "test [model...]",
	Short: "Run availability checks against one or more models",
	Long:  "Runs Anthropic streaming and/or tool-call checks against each named provider. Without arguments, tests every model.\n\nWarning: this issues real API calls against each provider's endpoint and may incur a tiny cost.",
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
		results := client.TestAll(ctx, models, nil)
		tester.PrintResults(os.Stdout, results)

		status, _ := tester.LoadStatus()
		if status == nil {
			status = &tester.StatusFile{Results: map[string]tester.ProviderStatus{}}
		}
		status.Upsert(results)
		if err := tester.SaveStatus(status); err != nil {
			fmt.Fprintln(os.Stderr, ui.Warn("warning: failed to persist test status: "+err.Error()))
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
}
