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
	Long: "Fill in the endpoint, fetch the model list, assign slots. " +
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

// the endpoint and startup rows above the slot rows.
const optionalHeaderRows = 5

func runOptional(paths store.Paths, cfg *config.Config, project ccstate.Project) error {
	seed := loadOptionalSeed(paths, cfg, project)
	values := seed.values

	header := []string{project.Dir}
	if seed.fromOptional {
		header = append(header, "editing the optional config this project already has")
	} else {
		header = append(header, "new optional config for this project")
	}

	var models []string
	errText := ""
	focus := 0

	for {
		res, err := workbench.RunPanel(workbench.PanelOptions{
			Title:  "optional",
			Header: header,
			Rows:   optionalRows(values),
			Focus:  focus,
			Error:  errText,
			Validate: func(v []string) error {
				_, err := groupFromOptional(v)
				return err
			},
			Footer: "enter choose   up/down move   ctrl+s apply   esc cancel",
		})
		if err != nil {
			return err
		}

		if res.PickRow >= 0 {
			values = res.Values
			focus = res.PickRow
			choice, ok, err := optionalPick(res.PickRow, values, &models)
			if err != nil {
				errText = err.Error()
				continue
			}
			if ok {
				values[res.PickRow] = choice
			}
			errText = ""
			continue
		}
		if !res.Saved {
			return nil
		}
		values = res.Values

		g, err := groupFromOptional(values)
		if err != nil {
			return err
		}
		startup := group.SlotName(strings.TrimSpace(values[4]))
		r, err := apply.Run(apply.Request{
			Paths:   paths,
			Group:   g,
			Layer:   apply.Project,
			Project: project,
			Options: render.Options{StartupSlot: startup},
			Action:  "optional.apply",
			Source:  "optional",
		})
		if err != nil {
			return err
		}
		printReceipt(r)
		return nil
	}
}

// optionalPick opens the right picker for a row: the four slot names for the
// startup row, the fetched model list for the model rows.
func optionalPick(row int, values []string, models *[]string) (string, bool, error) {
	if row == 4 {
		return workbench.RunPicker("fallback slot", []string{"fable", "opus", "sonnet", "haiku"}, values[4])
	}

	if len(*models) == 0 {
		fetched, err := fetchModels(values[0], values[1])
		if err != nil {
			return "", false, err
		}
		*models = fetched
	}
	slotIndex := row - optionalHeaderRows
	title := string(group.AllSlots[slotIndex]) + " model"
	return workbench.RunPicker(title, *models, values[row])
}

func fetchModels(baseURL, token string) ([]string, error) {
	baseURL = strings.TrimSpace(baseURL)
	token = strings.TrimSpace(token)
	if baseURL == "" {
		return nil, fmt.Errorf("fill in the base url first")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	snap, err := gateway.New(baseURL, token).Fetch(ctx)
	if err != nil {
		return nil, err
	}
	// new-api translates protocols, so every listed model is a candidate.
	ids := make([]string, 0, len(snap.Models))
	for _, m := range snap.Models {
		ids = append(ids, m.ID)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("%s returned no models", baseURL)
	}
	fmt.Printf("%d models fetched from %s\n", len(ids), baseURL)
	return ids, nil
}

func optionalRows(values []string) []workbench.Row {
	rows := []workbench.Row{
		{Section: "endpoint", Label: "base url", Kind: workbench.RowText,
			Value: values[0], Empty: "https://gateway.example.com"},
		{Label: "token", Kind: workbench.RowSecret, Value: values[1], Empty: "no token set"},
		{Label: "context", Kind: workbench.RowText, Value: values[2],
			Empty: "not set", Hint: "500k, 1m"},
		{Label: "auto-compact", Kind: workbench.RowText, Value: values[3],
			Empty: "90% of the context"},
		{Section: "startup", Label: "fallback", Kind: workbench.RowPick,
			Value: values[4], Empty: "sonnet"},
	}
	for i, name := range group.AllSlots {
		section := ""
		if i == 0 {
			section = "models"
		}
		rows = append(rows, workbench.Row{
			Section: section,
			Label:   string(name),
			Kind:    workbench.RowPick,
			Value:   values[optionalHeaderRows+i],
			Empty:   "not set",
		})
	}
	return rows
}

// optionalSeed is the starting state of the panel.
type optionalSeed struct {
	values       []string
	fromOptional bool
}

// loadOptionalSeed reopens on the configuration this project already has, but
// only when ccsc optional wrote it. A group applied with ccsc local is
// deliberately not inherited: optional is the place you build a configuration
// by hand.
func loadOptionalSeed(paths store.Paths, cfg *config.Config, project ccstate.Project) optionalSeed {
	blank := optionalSeed{values: []string{
		cfg.Defaults.BaseURL,
		cfg.Defaults.Token,
		group.FormatWindow(cfg.Defaults.Window),
		group.FormatWindow(cfg.Defaults.Compact),
		string(cfg.StartupSlot()),
		"", "", "", "", "",
	}}

	state, err := ccstate.LoadState(paths.Root)
	if err != nil {
		return blank
	}
	ps, ok := state.Projects[project.Key()]
	if !ok || ps.EffectiveSource() != "optional" {
		return blank
	}
	doc, err := ccstate.LoadDocument(project.LocalFile)
	if err != nil || !doc.Existed() {
		return blank
	}
	env := doc.Root.Object("env")
	get := func(key string) string {
		if env == nil {
			return ""
		}
		v, _ := env.String(key)
		return v
	}

	values := make([]string, optionalHeaderRows+len(group.AllSlots))
	values[0] = get(render.EnvBaseURL)
	values[1] = get(render.EnvToken)
	values[2] = group.FormatWindow(parseTokens(get(render.EnvMaxContextTokens)))
	if n, ok := doc.Root.Number("autoCompactWindow"); ok {
		values[3] = group.FormatWindow(n)
	}
	values[4] = ps.Startup
	if values[4] == "" {
		values[4] = string(cfg.StartupSlot())
	}
	for i, name := range group.AllSlots {
		values[optionalHeaderRows+i] = get(render.DefaultModelKey(name))
	}
	return optionalSeed{values: values, fromOptional: true}
}

func parseTokens(s string) int64 {
	n, _ := group.ParseWindow(s)
	return n
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
