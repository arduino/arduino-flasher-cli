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

	// One fetch, then filtered: nothing is cached, so asking per distribution
	// would only be more round trips for releases already in hand.
	all, err := registry.NewClient().Fetch(ctx)
	if err != nil {
		// One index failing should not hide the ones that did not.
		feedback.Warning(err.Error())
		if len(all) == 0 {
			feedback.Fatal(i18n.Tr("no image index could be read"), feedback.ErrGeneric)
		}
	}

	result := listResult{board: board}
	for _, r := range all.Filter(imageOs, board.ID) {
		result.Releases = append(result.Releases, listRelease{
			Image:  listImage{Version: r.Version, Board: r.Board, Os: r.OS},
			Latest: r.Latest,
			Url:    r.Url,
			Sha256: r.Sha256,
		})
	}

	feedback.PrintResult(result)
}

// The reported shape mirrors the daemon's List, so the two describe an image the
// same way.
type listResult struct {
	Releases []listRelease `json:"releases"`
	// board is only for the empty-result message.
	board registry.Board
}

type listRelease struct {
	Image listImage `json:"image"`
	// Latest is whether this is the most recent of its board and distribution.
	Latest bool `json:"latest"`
	// Url and Sha256 are where the archive is published, to fetch it without
	// going through the flash command.
	Url    string `json:"url"`
	Sha256 string `json:"sha256"`
}

// listImage identifies one image: all three parts are needed, since the same
// version can name a different image on another board or distribution.
type listImage struct {
	Version string `json:"version"`
	Board   string `json:"board"`
	// Os is the distribution as the index named it. Empty when it named none.
	Os string `json:"os,omitempty"`
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
		key := line{r.Image.Board, r.Image.Os}
		groups[key] = append(groups[key], r)
	}
	// By name rather than by which line has the newest image, so that publishing
	// one does not reshuffle the table.
	order := slices.SortedFunc(maps.Keys(groups), func(a, b line) int {
		return cmp.Or(strings.Compare(a.board, b.board), strings.Compare(a.os, b.os))
	})

	t := table.NewWriter()
	t.SetStyle(tablestyle.CustomCleanStyle)
	t.AppendHeader(table.Row{"BOARD", "OS", "VERSION", "LATEST"})
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
			// Named on every row: a row is what the flash arguments are read off.
			t.AppendRow(table.Row{key.board, key.os, r.Image.Version, mark})
		}
	}
	return t.Render()
}
