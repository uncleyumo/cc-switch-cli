package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"cc-switch-cli/internal/manager"
	"cc-switch-cli/internal/mcp"
	"cc-switch-cli/internal/model"
	"cc-switch-cli/internal/ui"
)

var localShowReveal bool

var localShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the project-level model and MCP configuration",
	Long: `Summarize what Claude Code will pick up for this project:

  • .claude/settings.local.json  — project-local model / env override
  • .mcp.json                    — project-scope MCP servers
  • ~/.claude.json projects.<cwd>.mcpServers — local-scope MCP servers

Secrets are masked unless --reveal is set. --path is inherited from local.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		snap, err := manager.InspectLocalConfig(manager.LocalOptions{Path: localPath})
		if err != nil {
			return err
		}
		printLocalSnapshot(snap)
		return nil
	},
}

func init() {
	localShowCmd.Flags().BoolVar(&localShowReveal, "reveal", false, "Show unmasked tokens")
}

func printLocalSnapshot(snap *manager.LocalSnapshot) {
	printKV("project", snap.ProjectDir)
	printKV("settings", snap.SettingsPath)
	printKV("status", localSettingsStatus(snap))

	if snap.Model != nil && (snap.HasModelOverride || len(snap.OtherKeys) > 0 || len(snap.ExtraEnv) > 0) {
		printLocalModel(snap)
	}

	fmt.Println()
	printLocalMCPBlock("project MCP  : "+displayPathOrNone(snap.ProjectMCPPath, len(snap.ProjectMCP) > 0), snap.ProjectMCP)
	printLocalMCPBlock("local MCP    : ~/.claude.json → this project", snap.LocalMCP)
}

func localSettingsStatus(snap *manager.LocalSnapshot) string {
	if !snap.SettingsExists {
		return ui.Dim("(none)")
	}
	if snap.ParseError != "" {
		return ui.Fail("present (invalid JSON: " + snap.ParseError + ")")
	}
	if !snap.HasModelOverride {
		if len(snap.OtherKeys) == 0 && len(snap.ExtraEnv) == 0 {
			return ui.Dim("present (empty)")
		}
		return "present " + ui.Dim("(no model override)")
	}
	return ui.Success("present")
}

func printLocalModel(snap *manager.LocalSnapshot) {
	m := snap.Model
	printKV("model", dash(m.TopLevelModel()))
	printKV("group", localGroupDisplay(snap))
	printKV("env model", dash(m.Env.Model))
	if haiku := m.Env.DefaultHaikuModel; haiku != "" {
		printKV("default haiku", haiku)
	}
	if opus := m.Env.DefaultOpusModel; opus != "" {
		printKV("default opus", opus)
	}
	if sonnet := m.Env.DefaultSonnetModel; sonnet != "" {
		printKV("default sonnet", sonnet)
	}
	printKV("base URL", dash(m.BaseURL()))
	printKV("token", localTokenDisplay(m))
	printKV("window", localWindowDisplay(m))
	if len(snap.OtherKeys) > 0 {
		printKV("other keys", strings.Join(snap.OtherKeys, ", "))
	}
	if len(snap.ExtraEnv) > 0 {
		printKV("extra env", strings.Join(snap.ExtraEnv, ", "))
	}
}

func localGroupDisplay(snap *manager.LocalSnapshot) string {
	switch len(snap.MatchedProviders) {
	case 0:
		return ui.Dim("(no matching group)")
	case 1:
		return ui.Success(snap.MatchedProviders[0])
	default:
		return strings.Join(snap.MatchedProviders, ", ")
	}
}

func localTokenDisplay(m *model.Model) string {
	tok := m.Token()
	if tok == "" {
		return "-"
	}
	if localShowReveal {
		return tok
	}
	return ui.MaskToken(tok)
}

func localWindowDisplay(m *model.Model) string {
	if n := m.Env.ContextWindowTokens(); n > 0 {
		return model.FormatContextWindow(n)
	}
	return "-"
}

func printLocalMCPBlock(header string, entries []mcp.Entry) {
	fmt.Println(header)
	if len(entries) == 0 {
		fmt.Println("  " + ui.Dim("(none)"))
		return
	}
	for _, e := range entries {
		fmt.Printf("  %-24s %-7s %s\n", e.Name, e.Server.Transport(), mcpEndpointSummary(e.Server))
	}
}

func printKV(key, val string) {
	fmt.Printf("%-14s: %s\n", key, val)
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func displayPathOrNone(path string, present bool) string {
	if present {
		return path
	}
	return path + " " + ui.Dim("(none)")
}
