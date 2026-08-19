package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"cc-switch-cli/internal/manager"
	"cc-switch-cli/internal/model"
	"cc-switch-cli/internal/ui"
)

// modelDoctorCmd reports, for every provider, which model id will actually be
// sent to the API ("wire") versus what is stored in the config. It is purely
// diagnostic and never rewrites files: context-window markers such as "[1m]"
// are intentionally kept in the config and stripped only on the wire, so they
// are reported as normal rather than as something to "fix".
var modelDoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Show the effective and on-the-wire model id per provider",
	Long: "For each provider, prints the configured model id, the id after tier-alias\n" +
		"resolution, the id actually sent in the request body (\"wire\"), and the\n" +
		"declared group context window.\n\n" +
		"Set the group default with the context-window field in `model edit` (128k /\n" +
		"256k / 500k / 1m / 2m). A trailing [1m] on an unrecognized id assumes 1M and ignores\n" +
		"CLAUDE_CODE_MAX_CONTEXT_TOKENS for that id; unmarked siblings keep the group window.\n" +
		"The marker is kept in the config and stripped automatically before any\n" +
		"request is sent. This command does not modify any file.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		names, err := manager.ListModels()
		if err != nil {
			return err
		}
		if len(names) == 0 {
			fmt.Println(ui.Dim("no models found"))
			return nil
		}

		anyEsc := false
		for _, n := range names {
			m, err := model.LoadByName(n)
			if err != nil {
				fmt.Println(ui.Fail("skip " + n + ": " + err.Error()))
				continue
			}
			resolved := m.ResolvedModelID()
			wire := m.WireModelID()
			window := m.Env.ContextWindowTokens()
			windowLabel := "-"
			if window > 0 {
				windowLabel = model.FormatContextWindow(window)
			}
			line := fmt.Sprintf("  %-20s  resolved %-26s  wire %-24s  window %s",
				ui.Bold(n), quoteOrDash(resolved), quoteOrDash(wire), windowLabel)
			fmt.Println(line)

			if rawEsc(resolved) {
				anyEsc = true
				fmt.Println("    " + ui.Warn("⚠ contains a raw ANSI escape byte (real terminal artifact) — edit this model and re-type the model id by hand"))
			} else if resolved != "" && resolved != wire {
				fmt.Println("    " + ui.Dim("[1m]/[Nk] kept in config, stripped on the wire (normal)"))
			} else if resolved == "" {
				fmt.Println("    " + ui.Warn("no model id could be resolved from this config"))
			}

			printWindowAdvice(m, resolved, window)
		}

		fmt.Println()
		if anyEsc {
			fmt.Println(ui.Warn("one or more fields contain a raw escape byte; fix those by hand with `model edit`."))
		} else {
			fmt.Println(ui.Success(fmt.Sprintf("all %d models look consistent (context markers like [1m] are preserved and stripped on the wire).", len(names))))
		}
		return nil
	},
}

func printWindowAdvice(m *model.Model, resolved string, window int) {
	if resolved == "" {
		return
	}
	unrecognized := !model.LooksLikeClaudeModel(resolved)
	if m.HasAny1MMarker() && window > 0 {
		fmt.Println("    " + ui.Dim("group window " + model.FormatContextWindow(window) + "; ids with [1m] assume 1M and ignore MAX_CONTEXT"))
		return
	}
	if unrecognized && !model.Has1MMarker(resolved) && window == 0 {
		fmt.Println("    " + ui.Dim("unrecognized id, no declared window — Claude Code assumes 200k unless you set context window"))
	}
}

// rawEsc reports whether s contains a genuine ASCII ESC byte (0x1b), which is
// the only thing we consider a real terminal artifact. The literal text "[1m]"
// has no ESC byte and is therefore not flagged.
func rawEsc(s string) bool { return strings.ContainsRune(s, '\x1b') }

func quoteOrDash(s string) string {
	if s == "" {
		return "-"
	}
	return fmt.Sprintf("%q", s)
}
