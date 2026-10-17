// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package cache

import (
	"os"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/spf13/cobra"

	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
	imagecache "github.com/arduino/arduino-flasher-cli/internal/cache"
	"github.com/arduino/arduino-flasher-cli/internal/tablestyle"
)

// NewCacheCmd creates the commands for inspecting and clearing downloaded images.
func NewCacheCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cache",
		Short: "Inspect and clear cached Linux images",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newListCmd(), newClearCmd())
	return cmd
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List cached images and their disk usage",
		Long: `List locally cached images without accessing the network.

Shows the board, distribution, version, archive size and download date.
Total cache size includes metadata and incomplete downloads. Archive checksums
are verified before reuse by download or flash, rather than while listing.
`,
		Example: " " + os.Args[0] + " cache list\n" +
			" " + os.Args[0] + " cache list --format json\n",
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			c, err := imagecache.Default()
			if err != nil {
				feedback.Fatal(i18n.Tr("error opening the image cache: %v", err), feedback.ErrGeneric)
			}
			inventory, err := c.List(cmd.Context())
			if err != nil {
				feedback.Fatal(i18n.Tr("error listing the image cache: %v", err), feedback.ErrGeneric)
			}
			feedback.PrintResult(listResult{
				Directory: c.Dir(), Images: inventory.Images, TotalSize: inventory.TotalSize,
			})
		},
	}
}

func newClearCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "clear",
		Short:   "Remove all cached images and incomplete downloads",
		Long:    "Remove the contents of the image cache. Downloads saved with --dest-dir are left in their chosen directory.",
		Example: " " + os.Args[0] + " cache clear\n",
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			c, err := imagecache.Default()
			if err != nil {
				feedback.Fatal(i18n.Tr("error opening the image cache: %v", err), feedback.ErrGeneric)
			}
			if err := c.Clear(cmd.Context()); err != nil {
				feedback.Fatal(i18n.Tr("error clearing the image cache: %v", err), feedback.ErrGeneric)
			}
			feedback.PrintResult(clearResult{Directory: c.Dir(), Cleared: true})
		},
	}
}

type listResult struct {
	Directory string             `json:"directory"`
	Images    []imagecache.Entry `json:"images"`
	TotalSize uint64             `json:"total_size"`
}

func (r listResult) Data() interface{} {
	return r
}

func (r listResult) String() string {
	header := i18n.Tr("Cache directory: %s", r.Directory)
	footer := i18n.Tr("Total cache size: %s", humanize.IBytes(r.TotalSize))
	if len(r.Images) == 0 {
		return header + "\n\n" + i18n.Tr("No complete images are cached.") + "\n\n" + footer
	}
	t := table.NewWriter()
	t.SetStyle(tablestyle.CustomCleanStyle)
	t.AppendHeader(table.Row{"BOARD", "OS", "VERSION", "SIZE", "DOWNLOADED"})
	t.SetColumnConfigs([]table.ColumnConfig{
		{Name: "BOARD", Align: text.AlignLeft},
		{Name: "OS", Align: text.AlignLeft},
		{Name: "VERSION", Align: text.AlignLeft},
		{Name: "SIZE", Align: text.AlignRight},
		{Name: "DOWNLOADED", Align: text.AlignLeft},
	})
	for _, image := range r.Images {
		// List returns only non-negative archive sizes.
		size := humanize.IBytes(uint64(image.Size)) //nolint:gosec
		t.AppendRow(table.Row{image.Board, image.OS, image.Version, size, image.DownloadedAt.UTC().Format(time.RFC3339)})
	}
	return header + "\n\n" + t.Render() + "\n\n" + footer
}

type clearResult struct {
	Directory string `json:"directory"`
	Cleared   bool   `json:"cleared"`
}

func (r clearResult) Data() interface{} {
	return r
}

func (r clearResult) String() string {
	return i18n.Tr("Image cache cleared: %s", r.Directory)
}
