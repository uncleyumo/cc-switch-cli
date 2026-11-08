package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/uncleyumo/ccsc/internal/apply"
	"github.com/uncleyumo/ccsc/internal/config"
	"github.com/uncleyumo/ccsc/internal/group"
	"github.com/uncleyumo/ccsc/internal/migrate"
	"github.com/uncleyumo/ccsc/internal/render"
	"github.com/uncleyumo/ccsc/internal/store"
	"github.com/uncleyumo/ccsc/internal/workbench"
)

var groupCmd = &cobra.Command{
	Use:   "group",
	Short: "manage the group library",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, cfg, err := load()
		if err != nil {
			return err
		}
		if err := requireTTY(); err != nil {
			return err
		}
		return runGroups(paths, cfg)
	},
}

func runGroups(paths store.Paths, cfg *config.Config) error {
	status := ""
	for {
		names, err := group.List(paths.GroupsDir())
		if err != nil {
			return err
		}
		res, err := workbench.RunList(workbench.ListOptions{
			Title:  "groups",
			Items:  names,
			Header: []string{store.Shorten(paths.GroupsDir())},
			Bindings: []workbench.Binding{
				{Key: "enter", Label: "edit", Action: workbench.ActEdit},
				{Key: "n", Label: "new", Action: workbench.ActNew},
				{Key: "u", Label: "apply globally", Action: workbench.ActApply},
				{Key: "d", Label: "delete", Action: workbench.ActDelete},
				{Key: "q", Label: "quit", Action: workbench.ActQuit},
			},
			Status: status,
			Empty:  "no groups yet, press n to create one",
		})
		if err != nil {
			return err
		}
		status = ""

		switch res.Action {
		case workbench.ActQuit:
			return nil
		case workbench.ActNew:
			if err := editGroup(paths, cfg, nil); err != nil {
				status = err.Error()
			}
		case workbench.ActEdit:
			g, err := group.Load(paths.GroupsDir(), res.Item)
			if err != nil {
				status = err.Error()
				continue
			}
			if err := editGroup(paths, cfg, g); err != nil {
				status = err.Error()
			}
		case workbench.ActDelete:
			ok, err := workbench.Confirm(fmt.Sprintf("delete group %s?", res.Item),
				"delete, settings already written stay as they are", "cancel")
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			if err := group.Delete(paths.GroupsDir(), res.Item); err != nil {
				status = err.Error()
				continue
			}
			status = "deleted " + res.Item
		case workbench.ActApply:
			g, err := group.Load(paths.GroupsDir(), res.Item)
			if err != nil {
				status = err.Error()
				continue
			}
			r, err := apply.Run(apply.Request{
				Paths:   paths,
				Group:   g,
				Layer:   apply.Global,
				Options: render.Options{StartupSlot: cfg.StartupSlot()},
				Action:  "group.apply",
			})
			if err != nil {
				status = err.Error()
				continue
			}
			printReceipt(r)
			status = "applied " + res.Item + " globally"
		}
	}
}

// printReceipt writes a receipt to stdout and leaves the screen tidy.
func printReceipt(r interface{ Render() string }) {
	out := r.Render()
	if out == "" {
		return
	}
	fmt.Print(out)
	if !strings.HasSuffix(out, "\n") {
		fmt.Println()
	}
}

// field binds one panel row to one part of a group, so the row order and the
// parse order cannot drift apart.
type field struct {
	row workbench.Row
	set func(*group.Group, string) error
}

func groupFields(g *group.Group, isNew bool, cfg *config.Config) []field {
	var fs []field

	name := ""
	baseURL, token, window := cfg.Defaults.BaseURL, cfg.Defaults.Token, ""
	if cfg.Defaults.Window > 0 {
		window = group.FormatWindow(cfg.Defaults.Window)
	}
	if g != nil {
		name = g.Name
		baseURL = g.BaseURL
		token = g.Token
		window = group.FormatWindow(g.Window)
	}

	fs = append(fs, field{
		row: workbench.Row{Section: "group", Label: "name", Kind: workbench.RowText, Value: name, Empty: "cost-effective"},
		set: func(x *group.Group, v string) error {
			x.Name = v
			if v == "" {
				return fmt.Errorf("name is required")
			}
			return nil
		},
	})
	fs = append(fs,
		field{
			row: workbench.Row{Section: "endpoint", Label: "base url", Kind: workbench.RowText, Value: baseURL, Empty: "https://gateway.example.com"},
			set: func(x *group.Group, v string) error { x.BaseURL = v; return nil },
		},
		field{
			row: workbench.Row{Label: "token", Kind: workbench.RowSecret, Value: token, Empty: "no token set"},
			set: func(x *group.Group, v string) error { x.Token = v; return nil },
		},
		field{
			row: workbench.Row{Label: "context", Kind: workbench.RowText, Value: window, Empty: "not set", Hint: "500k, 1m"},
			set: func(x *group.Group, v string) error {
				n, err := group.ParseWindow(v)
				if err != nil {
					return err
				}
				x.Window = n
				return nil
			},
		},
	)

	for _, slotName := range group.AllSlots {
		slot := group.Slot{}
		if g != nil {
			slot = g.Slot(slotName)
		}
		name := slotName
		slotWindow := ""
		if slot.Is1M() {
			slotWindow = "1m"
		}
		compact := group.FormatWindow(slot.Compact)
		fs = append(fs,
			field{
				row: workbench.Row{Section: string(name), Label: "model", Kind: workbench.RowText, Value: slot.Model, Empty: "not set"},
				set: func(x *group.Group, v string) error {
					if x.Slots == nil {
						x.Slots = map[group.SlotName]group.Slot{}
					}
					cur := x.Slots[name]
					cur.Model = v
					x.Slots[name] = cur
					return nil
				},
			},
			field{
				row: workbench.Row{Label: "display name", Kind: workbench.RowText, Value: slot.Name, Empty: "not set"},
				set: func(x *group.Group, v string) error {
					if x.Slots == nil {
						x.Slots = map[group.SlotName]group.Slot{}
					}
					cur := x.Slots[name]
					cur.Name = v
					x.Slots[name] = cur
					return nil
				},
			},
			field{
				row: workbench.Row{Label: "window", Kind: workbench.RowText, Value: slotWindow, Empty: "group default", Hint: "only 1m or empty"},
				set: func(x *group.Group, v string) error {
					n, err := group.ParseWindow(v)
					if err != nil {
						return err
					}
					if n != 0 && n < group.OneMillion {
						return fmt.Errorf("a slot window can only be 1m; anything else goes on the group context")
					}
					if x.Slots == nil {
						x.Slots = map[group.SlotName]group.Slot{}
					}
					cur := x.Slots[name]
					cur.Window = n
					x.Slots[name] = cur
					return nil
				},
			},
			field{
				row: workbench.Row{Label: "auto-compact", Kind: workbench.RowText, Value: compact, Empty: "90% of the window"},
				set: func(x *group.Group, v string) error {
					n, err := group.ParseWindow(v)
					if err != nil {
						return err
					}
					if x.Slots == nil {
						x.Slots = map[group.SlotName]group.Slot{}
					}
					cur := x.Slots[name]
					cur.Compact = n
					x.Slots[name] = cur
					return nil
				},
			},
		)
	}
	return fs
}

func parseGroup(fs []field, values []string) (*group.Group, error) {
	g := &group.Group{Schema: group.SchemaVersion, Slots: map[group.SlotName]group.Slot{}}
	for i, f := range fs {
		if err := f.set(g, strings.TrimSpace(values[i])); err != nil {
			return nil, fmt.Errorf("%s: %w", f.row.Label, err)
		}
	}
	if err := g.Validate(); err != nil {
		return nil, err
	}
	return g, nil
}

func editGroup(paths store.Paths, cfg *config.Config, existing *group.Group) error {
	fs := groupFields(existing, existing == nil, cfg)
	rows := make([]workbench.Row, len(fs))
	for i, f := range fs {
		rows[i] = f.row
	}

	title := "new group"
	if existing != nil {
		title = "edit " + existing.Name
	}
	res, err := workbench.RunPanel(workbench.PanelOptions{
		Title: title,
		Rows:  rows,
		Validate: func(values []string) error {
			_, err := parseGroup(fs, values)
			return err
		},
	})
	if err != nil {
		return err
	}
	if !res.Saved {
		return nil
	}
	g, err := parseGroup(fs, res.Values)
	if err != nil {
		return err
	}
	if err := group.Save(paths.GroupsDir(), g); err != nil {
		return err
	}
	fmt.Printf("saved group %s\n", g.Name)
	return nil
}

// requireTTY refuses to hang waiting for keys in a pipeline.
func requireTTY() error {
	info, err := os.Stdin.Stat()
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeCharDevice == 0 {
		return fmt.Errorf("this command is interactive; run it in a terminal")
	}
	return nil
}

// load resolves the state root, migrates any v1 data once, and reads the
// settings panel.
func load() (store.Paths, *config.Config, error) {
	paths, err := store.New()
	if err != nil {
		return store.Paths{}, nil, err
	}
	if err := paths.Ensure(); err != nil {
		return store.Paths{}, nil, err
	}
	migrateOnce(paths)
	cfg, err := config.Load(paths)
	if err != nil {
		return store.Paths{}, nil, err
	}
	return paths, cfg, nil
}

// migrateOnce converts v1 provider files the first time ccsc v2 runs. The
// originals are moved into backups/, never deleted outright.
func migrateOnce(paths store.Paths) {
	if migrate.IsDone(paths.Root) {
		return
	}
	legacy := migrate.LegacyModelsDir(paths.Root)
	if _, err := os.Stat(legacy); err != nil {
		return
	}
	results, err := migrate.Scan(legacy, paths.GroupsDir())
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: v1 migration failed, originals left untouched: %v\n", err)
		return
	}
	var ok []string
	for _, r := range results {
		if r.Err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipped %s: %v\n", r.Name, r.Err)
			continue
		}
		if r.Group != nil {
			ok = append(ok, r.Name)
		}
	}
	archived, err := migrate.Archive(legacy, paths.BackupsDir())
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: migrated, but could not archive the originals: %v\n", err)
	}
	if err := migrate.MarkDone(paths.Root); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not write the migration marker: %v\n", err)
	}
	if len(ok) > 0 {
		fmt.Printf("migrated %d groups from v1: %s\n", len(ok), strings.Join(ok, ", "))
	}
	if archived != "" {
		fmt.Printf("  originals archived in %s\n", store.Shorten(archived))
	}
}

func init() {
	rootCmd.AddCommand(groupCmd)
	// `ccsc` with no arguments opens the group list.
	rootCmd.RunE = groupCmd.RunE
}
