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
	rows := []workbench.Row{
		{Section: "defaults", Label: "base url", Kind: workbench.RowText,
			Value: cfg.Defaults.BaseURL, Empty: "empty, filled per group"},
		{Label: "token", Kind: workbench.RowSecret, Value: cfg.Defaults.Token, Empty: "no default token"},
		{Label: "fallback slot", Kind: workbench.RowSuggest, Value: string(cfg.StartupSlot()),
			Suggest: []string{"fable", "opus", "sonnet", "haiku"}},
		{Label: "context", Kind: workbench.RowText, Value: group.FormatWindow(cfg.Defaults.Window),
			Empty: "not set", Hint: "500k, 1m"},
		{Label: "auto-compact", Kind: workbench.RowText, Value: group.FormatWindow(cfg.Defaults.Compact),
			Empty: "90% of the context"},
		{Section: "other", Label: "cache hours", Kind: workbench.RowText,
			Value: strconv.Itoa(cfg.Cache.TTLHours)},
		{Label: "theme", Kind: workbench.RowSuggest, Value: cfg.UI.Theme,
			Suggest: []string{"auto", "dark", "light"}},
		{Label: "settings file", Kind: workbench.RowText, Value: cfg.Target.Path,
			Empty: "~/.claude/settings.json"},
	}

	res, err := workbench.RunPanel(workbench.PanelOptions{
		Title: "config",
		Rows:  rows,
		Validate: func(values []string) error {
			_, _, _, _, err := parseConfig(values)
			return err
		},
	})
	if err != nil {
		return err
	}
	if !res.Saved {
		return nil
	}

	baseURL, token, window, compact, err := parseConfig(res.Values)
	if err != nil {
		return err
	}
	ttl, err := strconv.Atoi(strings.TrimSpace(res.Values[5]))
	if err != nil || ttl < 0 {
		return fmt.Errorf("cache hours must be a non-negative number")
	}

	cfg.Defaults.BaseURL = baseURL
	cfg.Defaults.Token = token
	cfg.Defaults.StartupSlot = strings.TrimSpace(res.Values[2])
	cfg.Defaults.Window = window
	cfg.Defaults.Compact = compact
	cfg.Cache.TTLHours = ttl
	cfg.UI.Theme = strings.TrimSpace(res.Values[6])
	cfg.Target.Path = strings.TrimSpace(res.Values[7])

	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := config.Save(paths, cfg); err != nil {
		return err
	}
	fmt.Printf("saved config %s\n", store.Shorten(paths.ConfigFile()))
	return nil
}

func parseConfig(values []string) (baseURL, token string, window, compact int64, err error) {
	baseURL = strings.TrimSpace(values[0])
	token = strings.TrimSpace(values[1])
	if baseURL != "" {
		if err := group.ValidateBaseURL(baseURL); err != nil {
			return "", "", 0, 0, err
		}
	}
	if window, err = group.ParseWindow(values[3]); err != nil {
		return "", "", 0, 0, fmt.Errorf("context: %w", err)
	}
	if compact, err = group.ParseWindow(values[4]); err != nil {
		return "", "", 0, 0, fmt.Errorf("auto-compact: %w", err)
	}
	if slot := strings.TrimSpace(values[2]); slot != "" && !group.SlotName(slot).Valid() {
		return "", "", 0, 0, fmt.Errorf("fallback slot must be fable, opus, sonnet or haiku")
	}
	return baseURL, token, window, compact, nil
}

func init() { rootCmd.AddCommand(configCmd) }
