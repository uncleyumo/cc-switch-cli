package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/uncleyumo/ccsc/internal/config"
	"github.com/uncleyumo/ccsc/internal/group"
	"github.com/uncleyumo/ccsc/internal/store"
	"github.com/uncleyumo/ccsc/internal/workbench"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "defaults used when creating a group",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, cfg, err := load()
		if err != nil {
			return err
		}
		if err := requireTTY(); err != nil {
			return err
		}
		return runConfig(paths, cfg)
	},
}

func runConfig(paths store.Paths, cfg *config.Config) error {
	values := []string{
		cfg.Defaults.BaseURL,
		cfg.Defaults.Token,
		string(cfg.StartupSlot()),
		group.FormatWindow(cfg.Defaults.Window),
		group.FormatWindow(cfg.Defaults.Compact),
		strconv.Itoa(cfg.Cache.TTLHours),
		cfg.UI.Theme,
		cfg.Target.Path,
	}

	focus := 0
	for {
		res, err := workbench.RunPanel(workbench.PanelOptions{
			Title: "config",
			Rows:  configRows(values),
			Focus: focus,
			Validate: func(v []string) error {
				_, _, _, _, _, err := parseConfig(v)
				return err
			},
			Footer: "enter choose   up/down move   ctrl+s save   esc cancel",
		})
		if err != nil {
			return err
		}
		if res.PickRow >= 0 {
			values = res.Values
			focus = res.PickRow
			var choice string
			var ok bool
			switch res.PickRow {
			case 2:
				choice, ok, err = workbench.RunPicker("fallback slot",
					[]string{"fable", "opus", "sonnet", "haiku"}, values[2])
			case 6:
				choice, ok, err = workbench.RunPicker("theme",
					[]string{"auto", "dark", "light"}, values[6])
			}
			if err != nil {
				return err
			}
			if ok {
				values[res.PickRow] = choice
			}
			continue
		}
		if !res.Saved {
			return nil
		}
		values = res.Values
		break
	}

	baseURL, token, window, compact, ttl, err := parseConfig(values)
	if err != nil {
		return err
	}

	cfg.Defaults.BaseURL = baseURL
	cfg.Defaults.Token = token
	cfg.Defaults.StartupSlot = strings.TrimSpace(values[2])
	cfg.Defaults.Window = window
	cfg.Defaults.Compact = compact
	cfg.Cache.TTLHours = ttl
	cfg.UI.Theme = strings.TrimSpace(values[6])
	cfg.Target.Path = strings.TrimSpace(values[7])

	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := config.Save(paths, cfg); err != nil {
		return err
	}
	fmt.Printf("saved config %s\n", store.Shorten(paths.ConfigFile()))
	return nil
}

func configRows(values []string) []workbench.Row {
	return []workbench.Row{
		{Section: "defaults", Label: "base url", Kind: workbench.RowText,
			Value: values[0], Empty: "empty, filled per group"},
		{Label: "token", Kind: workbench.RowSecret, Value: values[1], Empty: "no default token"},
		{Label: "fallback", Kind: workbench.RowPick, Value: values[2], Empty: "sonnet"},
		{Label: "context", Kind: workbench.RowText, Value: values[3],
			Empty: "not set", Hint: "500k, 1m"},
		{Label: "auto-compact", Kind: workbench.RowText, Value: values[4],
			Empty: "90% of the context"},
		{Section: "other", Label: "cache hours", Kind: workbench.RowText, Value: values[5]},
		{Label: "theme", Kind: workbench.RowPick, Value: values[6], Empty: "auto"},
		{Label: "settings file", Kind: workbench.RowText, Value: values[7],
			Empty: "~/.claude/settings.json"},
	}
}

func parseConfig(values []string) (baseURL, token string, window, compact int64, ttl int, err error) {
	baseURL = strings.TrimSpace(values[0])
	token = strings.TrimSpace(values[1])
	if baseURL != "" {
		if err := group.ValidateBaseURL(baseURL); err != nil {
			return "", "", 0, 0, 0, err
		}
	}
	if window, err = group.ParseWindow(values[3]); err != nil {
		return "", "", 0, 0, 0, fmt.Errorf("context: %w", err)
	}
	if compact, err = group.ParseWindow(values[4]); err != nil {
		return "", "", 0, 0, 0, fmt.Errorf("auto-compact: %w", err)
	}
	ttl, err = strconv.Atoi(strings.TrimSpace(values[5]))
	if err != nil || ttl < 0 {
		return "", "", 0, 0, 0, fmt.Errorf("cache hours must be a non-negative number")
	}
	if slot := strings.TrimSpace(values[2]); slot != "" && !group.SlotName(slot).Valid() {
		return "", "", 0, 0, 0, fmt.Errorf("fallback must be fable, opus, sonnet or haiku")
	}
	return baseURL, token, window, compact, ttl, nil
}

func init() { rootCmd.AddCommand(configCmd) }
