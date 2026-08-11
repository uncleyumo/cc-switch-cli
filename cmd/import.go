package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"cc-switch-cli/internal/transfer"
	"cc-switch-cli/internal/ui"
)

var (
	importSecretFile string
	importForce      bool
)

var importCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Import an encrypted ccsc export",
	Long: `Decrypt and apply a file produced by ccs export.

The envelope format is validated before the secret is requested. Existing
models and user-scope MCP servers are skipped unless --force is set.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := os.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", args[0], err)
		}
		env, err := transfer.CheckEnvelope(data)
		if err != nil {
			return err
		}
		fmt.Printf("export format %d from ccsc %s (%s)\n", env.Format, env.Version, env.Created)
		fmt.Printf("  models: %v\n", env.Manifest.Models)
		fmt.Printf("  mcps:   %v\n", env.Manifest.MCPs)

		secret, err := resolveImportSecret()
		if err != nil {
			return err
		}
		payload, err := transfer.Open(env, secret)
		if err != nil {
			return err
		}
		report, err := transfer.Apply(payload, transfer.ImportOptions{Force: importForce})
		if err != nil {
			return err
		}
		printImportReport(report)
		return nil
	},
}

func init() {
	importCmd.Flags().StringVar(&importSecretFile, "secret-file", "", "Read decryption secret from file (\"-\" = stdin)")
	importCmd.Flags().BoolVar(&importForce, "force", false, "Overwrite existing models and MCP servers")
}

func resolveImportSecret() (string, error) {
	if importSecretFile != "" {
		return transfer.SecretFromFile(importSecretFile)
	}
	if !ui.IsTTY() {
		return "", fmt.Errorf("no terminal for secret prompt; pass --secret-file")
	}
	return transfer.PromptSecret("Import secret: ", false)
}

func printImportReport(r *transfer.ImportReport) {
	if r == nil {
		return
	}
	fmt.Println(ui.Success(fmt.Sprintf("applied %d model(s), %d mcp server(s)",
		len(r.ModelsApplied), len(r.MCPsApplied))))
	if len(r.ModelsApplied) > 0 {
		fmt.Println("  models applied:", joinShort(r.ModelsApplied))
	}
	if len(r.ModelsSkipped) > 0 {
		fmt.Println(ui.Warn("  models skipped (exists, use --force): " + joinShort(r.ModelsSkipped)))
	}
	if len(r.MCPsApplied) > 0 {
		fmt.Println("  mcps applied:  ", joinShort(r.MCPsApplied))
	}
	if len(r.MCPsSkipped) > 0 {
		fmt.Println(ui.Warn("  mcps skipped (exists, use --force): " + joinShort(r.MCPsSkipped)))
	}
}
