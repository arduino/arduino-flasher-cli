// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package list

import (
	"cmp"
	"context"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/spf13/cobra"

	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
	"github.com/arduino/arduino-flasher-cli/internal/cache"
	"github.com/arduino/arduino-flasher-cli/internal/registry"
	"github.com/arduino/arduino-flasher-cli/internal/tablestyle"
)

func NewListCmd() *cobra.Command {
	var osStr string
	cmd := &cobra.Command{
		Use:   "list [board]",
		Short: "List the available Linux images",
		Long: `List the available Linux images.

Everything published is listed, grouped by board and distribution. The argument
narrows the result to one board, and --os to one distribution.
The CACHED column marks local archives whose metadata matches the online image.
Archive checksums are verified before reuse by flash, rather than while listing.
`,
		Args: cobra.MaximumNArgs(1),
		Example: " " + os.Args[0] + " list\n" +
			" " + os.Args[0] + " list unoq\n" +
			" " + os.Args[0] + " list unoq --os debian\n",
		Run: func(cmd *cobra.Command, args []string) {
			// No board lists every one of them.
			boardID := ""
			if len(args) > 0 {
				boardID = args[0]
			}
			runListCommand(cmd.Context(), boardID, osStr)
		},
	}
	cmd.Flags().StringVar(&osStr, "os", "", "Distribution to list. Leave empty for all of them")
	return cmd
}

func runListCommand(ctx context.Context, boardID string, imageOs string) {
	var board registry.Board
	if boardID != "" {
		var ok bool
		if board, ok = registry.BoardByID(boardID); !ok {
			feedback.Fatal(i18n.Tr("%s is not a board, use one of: %s", boardID, strings.Join(registry.BoardIDs(), ", ")), feedback.ErrBadArgument)
		}
	}

	// Fetch the manifest once, then filter the releases already in hand.
	all, err := registry.NewClient().Fetch(ctx)
	if err != nil {
		// One index failing should not hide the ones that did not.
		feedback.Warning(err.Error())
		if len(all) == 0 {
			feedback.Fatal(i18n.Tr("no image index could be read"), feedback.ErrGeneric)
		}
	}

	// Inspect local metadata once, without hashing multi-GB archives. An
	// unavailable cache should not hide images published in the online manifest.
	var inventory cache.Inventory
	imageCache, cacheErr := cache.Default()
	if cacheErr == nil {
		inventory, cacheErr = imageCache.List(ctx)
	}
	if cacheErr != nil {
		feedback.Warning(i18n.Tr("could not inspect the image cache: %v", cacheErr))
		inventory = cache.Inventory{}
	}

	result := listResult{board: board}
	for _, r := range all.Filter(imageOs, board.ID) {
		result.Releases = append(result.Releases, listRelease{
			Version: r.Version,
			Board:   r.Board,
			Os:      r.OS,
			Latest:  r.Latest,
			Cached:  inventory.Contains(r),
			Url:     r.Url,
			Sha256:  r.Sha256,
		})
	}

	feedback.PrintResult(result)
}

// listResult describes published images together with their local cache status.
type listResult struct {
	Releases []listRelease `json:"releases"`
	// board is only for the empty-result message.
	board registry.Board
}

// All three of version, board and os are needed to identify an image: the same
// version can name a different one on another board or distribution.
type listRelease struct {
	Version string `json:"version"`
	Board   string `json:"board"`
	// Os is the distribution as the index named it. Empty when it named none.
	Os string `json:"os,omitempty"`
	// Latest is whether this is the most recent of its board and distribution.
	Latest bool `json:"latest"`
	// Cached indicates matching local metadata, not a freshly verified checksum.
	Cached bool `json:"cached"`
	// Url and Sha256 are where the archive is published, to fetch it without
	// going through the flash command.
	Url    string `json:"url"`
	Sha256 string `json:"sha256"`
}

// Data implements Result interface
func (lr listResult) Data() interface{} {
	return lr
}

// String implements Result interface
func (lr listResult) String() string {
	if len(lr.Releases) == 0 {
		if lr.board.ID != "" {
			return i18n.Tr("No images are published for the %s yet.", lr.board.Label)
		}
		return i18n.Tr("No images are published yet.")
	}

	// Grouped per board and distribution, since that is the line a version
	// belongs to. Named the way flash and download take them, so a row reads
	// back into the next command.
	type line struct{ board, os string }
	groups := map[line][]listRelease{}
	for _, r := range lr.Releases {
		key := line{r.Board, r.Os}
		groups[key] = append(groups[key], r)
	}
	// By name rather than by which line has the newest image, so that publishing
	// one does not reshuffle the table.
	order := slices.SortedFunc(maps.Keys(groups), func(a, b line) int {
		return cmp.Or(strings.Compare(a.board, b.board), strings.Compare(a.os, b.os))
	})

	t := table.NewWriter()
	t.SetStyle(tablestyle.CustomCleanStyle)
	t.AppendHeader(table.Row{"BOARD", "OS", "VERSION", "LATEST", "CACHED"})
	// The style centers cells, which leaves names of different lengths ragged.
	// These are read off to be retyped, so they line up instead.
	t.SetColumnConfigs([]table.ColumnConfig{
		{Name: "BOARD", Align: text.AlignLeft},
		{Name: "OS", Align: text.AlignLeft},
		{Name: "VERSION", Align: text.AlignLeft},
	})
	for _, key := range order {
		for _, r := range groups[key] {
			mark := ""
			if r.Latest {
				mark = "✓"
			}
			cachedMark := ""
			if r.Cached {
				cachedMark = "✓"
			}
			// Named on every row: a row is what the flash arguments are read off.
			t.AppendRow(table.Row{key.board, key.os, r.Version, mark, cachedMark})
		}
	}
	return t.Render()
}
