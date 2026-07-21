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
		"resolution, and the id actually sent in the request body (\"wire\").\n\n" +
		"A trailing context-window marker such as \"[1m]\" or \"[200k]\" is a deliberate\n" +
		"annotation that selects the larger context window in Claude Code. It is kept\n" +
		"in the config on purpose and is stripped automatically before any request is\n" +
		"sent — so a \"[1m]\" suffix is normal and must not be removed. This command does\n" +
		"not modify any file; edit fields with `model edit` if you need to.",
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
			line := fmt.Sprintf("  %-20s  resolved %-26s  wire %s",
				ui.Bold(n), quoteOrDash(resolved), quoteOrDash(wire))
			fmt.Println(line)

			if rawEsc(resolved) {
				anyEsc = true
				fmt.Println("    " + ui.Warn("⚠ contains a raw ANSI escape byte (real terminal artifact) — edit this model and re-type the model id by hand"))
			} else if resolved != "" && resolved != wire {
				fmt.Println("    " + ui.Dim("[1m]/[200k] context marker kept in config, stripped on the wire (normal)"))
			} else if resolved == "" {
				fmt.Println("    " + ui.Warn("no model id could be resolved from this config"))
			}
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
