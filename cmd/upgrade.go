package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/uncleyumo/ccsc/internal/update"
	"github.com/uncleyumo/ccsc/internal/version"
)

var upgradeCheck bool

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "update ccsc from GitHub Releases",
	Long:  "download the release for this platform, verify sha256, move the current binary aside as a date-stamped .bak, then install the new version and self-check.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		rel, err := update.Latest(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("current %s\nlatest  %s\n\n", version.Version, rel.Tag)
		if !version.IsNewer(rel.Tag, version.Version) {
			fmt.Println("already up to date.")
			return nil
		}
		if upgradeCheck {
			fmt.Printf("%s is available; run ccsc upgrade to install.\n", rel.Tag)
			return nil
		}

		asset := update.AssetName()
		url, ok := rel.Assets[asset]
		if !ok {
			return fmt.Errorf("release has no %s; assets: %s", asset, assetNames(rel))
		}

		zipData, err := update.Download(ctx, url, 64<<20)
		if err != nil {
			return err
		}
		if checksumURL := update.ChecksumURL(rel); checksumURL != "" {
			sums, err := update.Download(ctx, checksumURL, 4<<20)
			if err != nil {
				return err
			}
			if err := update.VerifySHA256(zipData, sums, asset); err != nil {
				return err
			}
			fmt.Println("checksum ok")
		} else {
			fmt.Println("warning: release has no checksums.txt, skipping verification")
		}

		binary, err := update.Extract(zipData, update.BinaryName())
		if err != nil {
			return err
		}

		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("cannot locate the running binary: %w", err)
		}
		backup, err := update.Replace(exe, binary)
		if err != nil {
			return err
		}
		fmt.Printf("replaced %s\n  backup %s\n", exe, backup)

		if err := update.SelfCheck(10 * time.Second); err != nil {
			if restoreErr := restore(exe, backup); restoreErr != nil {
				return fmt.Errorf("self-check failed (%v) and rollback failed (%v); backup kept at %s", err, restoreErr, backup)
			}
			return fmt.Errorf("self-check failed (%v), rolled back to the previous version", err)
		}
		fmt.Println("self-check ok")
		return nil
	},
}

func restore(exe, backup string) error {
	if err := os.Remove(exe); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(backup, exe)
}

func assetNames(rel *update.Release) string {
	names := make([]string, 0, len(rel.Assets))
	for name := range rel.Assets {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func init() {
	upgradeCmd.Flags().BoolVar(&upgradeCheck, "check", false, "only check for a new version")
	rootCmd.AddCommand(upgradeCmd)
}
