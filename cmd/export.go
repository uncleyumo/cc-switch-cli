package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"cc-switch-cli/internal/mcp"
	"cc-switch-cli/internal/model"
	"cc-switch-cli/internal/transfer"
	"cc-switch-cli/internal/tui"
	"cc-switch-cli/internal/ui"
)

var (
	exportOut        string
	exportModelsFlag string
	exportMCPsFlag   string
	exportSecretFile string
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export selected models and user-scope MCP servers (encrypted)",
	Long: `Build an encrypted export of the model library and/or user-scope MCP servers.

In a terminal with no --models/--mcps flags, opens a checklist selector.
Otherwise --models and --mcps accept "all", "none" or a comma-separated list.

The secret is prompted interactively (confirmed), or read from --secret-file
("-" = stdin). The resulting file can be imported with ccs import.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		modelNames, err := model.List()
		if err != nil {
			return err
		}
		mcpEntries, err := mcp.List(mcp.ScopeUser, "")
		if err != nil {
			return err
		}
		mcpNames := make([]string, 0, len(mcpEntries))
		for _, e := range mcpEntries {
			mcpNames = append(mcpNames, e.Name)
		}

		selModels, selMCPs, err := resolveExportSelection(modelNames, mcpNames)
		if err != nil {
			return err
		}
		if len(selModels) == 0 && len(selMCPs) == 0 {
			return fmt.Errorf("nothing selected to export")
		}

		secret, err := resolveSecret(exportSecretFile, true)
		if err != nil {
			return err
		}

		payload, err := transfer.BuildPayload(selModels, selMCPs)
		if err != nil {
			return err
		}
		env, err := transfer.Seal(payload, secret)
		if err != nil {
			return err
		}
		data, err := json.MarshalIndent(env, "", "  ")
		if err != nil {
			return err
		}
		data = append(data, '\n')

		out := exportOut
		if out == "" {
			out = fmt.Sprintf("ccsc-export-%s.json", time.Now().Format("20060102-150405"))
		}
		if err := os.WriteFile(out, data, 0o600); err != nil {
			return fmt.Errorf("failed to write %s: %w", out, err)
		}
		fmt.Println(ui.Success(fmt.Sprintf("exported %d model(s), %d mcp server(s) -> %s",
			len(selModels), len(selMCPs), out)))
		if len(selModels) > 0 {
			fmt.Println("  models:", joinShort(selModels))
		}
		if len(selMCPs) > 0 {
			fmt.Println("  mcps:  ", joinShort(selMCPs))
		}
		return nil
	},
}

func init() {
	exportCmd.Flags().StringVarP(&exportOut, "output", "o", "", "Output file (default ccsc-export-<timestamp>.json)")
	exportCmd.Flags().StringVar(&exportModelsFlag, "models", "", "Models to export: all, none, or comma-separated names")
	exportCmd.Flags().StringVar(&exportMCPsFlag, "mcps", "", "MCP servers to export: all, none, or comma-separated names")
	exportCmd.Flags().StringVar(&exportSecretFile, "secret-file", "", "Read encryption secret from file (\"-\" = stdin)")
}

func resolveExportSelection(availableModels, availableMCPs []string) (models, mcps []string, err error) {
	modelsFlagSet := exportModelsFlag != ""
	mcpsFlagSet := exportMCPsFlag != ""

	if !modelsFlagSet && !mcpsFlagSet && ui.IsTTY() {
		models, mcps, ok, err := tui.RunExportSelector(availableModels, availableMCPs)
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			return nil, nil, fmt.Errorf("cancelled")
		}
		return models, mcps, nil
	}

	// Non-interactive defaults: both "all" when flags are absent.
	if !modelsFlagSet {
		exportModelsFlag = "all"
	}
	if !mcpsFlagSet {
		exportMCPsFlag = "all"
	}
	models, err = transfer.ParseSelection(exportModelsFlag, availableModels)
	if err != nil {
		return nil, nil, fmt.Errorf("--models: %w", err)
	}
	mcps, err = transfer.ParseSelection(exportMCPsFlag, availableMCPs)
	if err != nil {
		return nil, nil, fmt.Errorf("--mcps: %w", err)
	}
	return models, mcps, nil
}

func resolveSecret(file string, confirm bool) (string, error) {
	if file != "" {
		return transfer.SecretFromFile(file)
	}
	if !ui.IsTTY() {
		return "", fmt.Errorf("no terminal for secret prompt; pass --secret-file")
	}
	prompt := "Export secret: "
	if confirm {
		// PromptSecret with confirm re-prompts with the same string; make it clear.
		s, err := transfer.PromptSecret("Export secret: ", false)
		if err != nil {
			return "", err
		}
		again, err := transfer.PromptSecret("Confirm secret: ", false)
		if err != nil {
			return "", err
		}
		if again != s {
			return "", fmt.Errorf("secrets do not match")
		}
		if len(s) < 8 {
			return "", fmt.Errorf("secret must be at least 8 characters")
		}
		return s, nil
	}
	return transfer.PromptSecret(prompt, false)
}

func joinShort(names []string) string {
	if len(names) <= 8 {
		out := ""
		for i, n := range names {
			if i > 0 {
				out += ", "
			}
			out += n
		}
		return out
	}
	return fmt.Sprintf("%s, %s, ... (%d total)", names[0], names[1], len(names))
}
