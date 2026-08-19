// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package list

import (
	"context"
	"os"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
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

Every index is listed. The argument narrows the result to one board, and --os to
one distribution.
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

func runListCommand(ctx context.Context, boardID, imageOs string) {
	var board registry.Board
	if boardID != "" {
		var ok bool
		if board, ok = registry.BoardByID(boardID); !ok {
			feedback.Fatal(i18n.Tr("%s is not a board, use one of: %s", boardID, strings.Join(registry.BoardIDs(), ", ")), feedback.ErrBadArgument)
		}
	}

	indexes := registry.Indexes()
	if imageOs != "" {
		indexes = []string{imageOs}
	}

	client := registry.NewClient()
	result := listResult{Latest: map[string]registry.Release{}, board: board}
	failed := 0
	for _, index := range indexes {
		manifest, err := client.GetInfoManifest(ctx, index)
		if err != nil {
			// One index being unreachable should not hide the others.
			feedback.Warning(i18n.Tr("error retrieving the %s manifest: %v", index, err))
			failed++
			continue
		}
		result.Latest[index] = manifest.Latest
		for _, r := range manifest.Releases {
			// An index names a board by its label.
			if board.ID == "" || r.Board == board.Label {
				result.Releases = append(result.Releases, r)
			}
		}
	}
	if failed == len(indexes) {
		feedback.Fatal(i18n.Tr("no image index could be retrieved"), feedback.ErrGeneric)
	}

	feedback.PrintResult(result)
}

type listResult struct {
	// Latest is each index's own most recent release, not necessarily the most
	// recent one for the board asked for.
	Latest   map[string]registry.Release `json:"latest"`
	Releases []registry.Release          `json:"releases"`
	// board is only for the empty-result message.
	board registry.Board
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

	isLatest := func(r registry.Release) bool {
		for _, latest := range lr.Latest {
			if r.Url != "" && latest.Url == r.Url {
				return true
			}
		}
		return false
	}

	t := table.NewWriter()
	t.SetStyle(tablestyle.CustomCleanStyle)
	t.AppendHeader(table.Row{"VERSION", "BOARD", "DISTRIBUTION", "LATEST"})

	for i := len(lr.Releases) - 1; i >= 0; i-- {
		r := lr.Releases[i]
		row := table.Row{r.Version, r.Board, r.Distro}
		if isLatest(r) {
			row = append(row, "✓")
		}
		t.AppendRow(row)
	}
	return t.Render()
}
