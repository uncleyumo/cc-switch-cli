package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/uncleyumo/ccsc/internal/apply"
	"github.com/uncleyumo/ccsc/internal/ccstate"
	"github.com/uncleyumo/ccsc/internal/config"
	"github.com/uncleyumo/ccsc/internal/gateway"
	"github.com/uncleyumo/ccsc/internal/group"
	"github.com/uncleyumo/ccsc/internal/render"
	"github.com/uncleyumo/ccsc/internal/store"
	"github.com/uncleyumo/ccsc/internal/workbench"
)

var optionalCmd = &cobra.Command{
	Use:   "optional",
	Short: "connect to a gateway now, write the result into this project",
	Long: "One panel: point ccsc at a new-api endpoint, fetch the model list, assign slots. " +
		"The result is written into the project and stays in effect until ccsc local or ccsc local clear overrides it.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, cfg, err := load()
		if err != nil {
			return err
		}
		project, ok := resolveProject(projectFlag)
		if !ok {
			return fmt.Errorf("%s is not a project directory", projectFlag)
		}
		if err := requireTTY(); err != nil {
			return err
		}
		return runOptional(paths, cfg, project)
	},
}

// fixed rows above the slot rows.
const optionalHeaderRows = 5

func runOptional(paths store.Paths, cfg *config.Config, project ccstate.Project) error {
	rows := []workbench.Row{
		{Section: "endpoint", Label: "base url", Kind: workbench.RowText,
			Value: cfg.Defaults.BaseURL, Empty: "https://gateway.example.com"},
		{Label: "token", Kind: workbench.RowSecret, Value: cfg.Defaults.Token, Empty: "no token set"},
		{Label: "context", Kind: workbench.RowText, Value: group.FormatWindow(cfg.Defaults.Window),
			Empty: "not set", Hint: "500k, 1m"},
		{Label: "auto-compact", Kind: workbench.RowText, Value: group.FormatWindow(cfg.Defaults.Compact),
			Empty: "90% of the context"},
		{Section: "startup", Label: "fallback", Kind: workbench.RowSuggest, Value: string(cfg.StartupSlot()),
			Suggest: []string{"fable", "opus", "sonnet", "haiku"}},
	}
	for i, name := range group.AllSlots {
		section := ""
		if i == 0 {
			section = "models"
		}
		rows = append(rows, workbench.Row{
			Section: section,
			Label:   string(name),
			Kind:    workbench.RowSuggest,
			Empty:   "not set",
		})
	}

	res, err := workbench.RunPanel(workbench.PanelOptions{
		Title:  "optional",
		Header: []string{project.Dir},
		Rows:   rows,
		Validate: func(values []string) error {
			_, err := groupFromOptional(values)
			return err
		},
		Fetch: func(values []string) ([]string, string, error) {
			baseURL := strings.TrimSpace(values[0])
			token := strings.TrimSpace(values[1])
			if baseURL == "" {
				return nil, "", fmt.Errorf("fill in the base url first")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			snap, err := gateway.New(baseURL, token).Fetch(ctx)
			if err != nil {
				return nil, "", err
			}
			// new-api translates protocols, so every listed model is a
			// candidate. Do not filter by supported_endpoint_types.
			ids := make([]string, 0, len(snap.Models))
			for _, m := range snap.Models {
				ids = append(ids, m.ID)
			}
			return ids, fmt.Sprintf("%d models fetched", len(ids)), nil
		},
		Footer: "ctrl+f fetch models   up/down move   left/right previous/next   enter accept & next   ctrl+s apply   esc cancel",
	})
	if err != nil {
		return err
	}
	if !res.Saved {
		return nil
	}

	g, err := groupFromOptional(res.Values)
	if err != nil {
		return err
	}
	startup := group.SlotName(strings.TrimSpace(res.Values[4]))
	r, err := apply.Run(apply.Request{
		Paths:   paths,
		Group:   g,
		Layer:   apply.Project,
		Project: project,
		Options: render.Options{StartupSlot: startup},
		Action:  "optional.apply",
	})
	if err != nil {
		return err
	}
	printReceipt(r)
	return nil
}

// groupFromOptional turns the panel values into a group bound for the project
// layer. The single auto-compact value is pushed onto every slot so the
// project layer writes exactly one top-level window.
func groupFromOptional(values []string) (*group.Group, error) {
	baseURL := strings.TrimSpace(values[0])
	token := strings.TrimSpace(values[1])
	if baseURL == "" {
		return nil, fmt.Errorf("base url is required")
	}
	if err := group.ValidateBaseURL(baseURL); err != nil {
		return nil, err
	}
	if token == "" {
		return nil, fmt.Errorf("token is required")
	}
	window, err := group.ParseWindow(values[2])
	if err != nil {
		return nil, fmt.Errorf("context: %w", err)
	}
	compact, err := group.ParseWindow(values[3])
	if err != nil {
		return nil, fmt.Errorf("auto-compact: %w", err)
	}

	g := &group.Group{
		Schema:  group.SchemaVersion,
		Name:    "optional",
		BaseURL: group.NormalizeBaseURL(baseURL),
		Token:   token,
		Window:  window,
		Slots:   map[group.SlotName]group.Slot{},
	}
	for i, name := range group.AllSlots {
		model := strings.TrimSpace(values[optionalHeaderRows+i])
		if model == "" {
			continue
		}
		g.Slots[name] = group.Slot{Model: model, Compact: compact}
	}
	if err := g.Validate(); err != nil {
		return nil, err
	}
	return g, nil
}

func init() {
	addProjectFlag(optionalCmd)
	rootCmd.AddCommand(optionalCmd)
}
