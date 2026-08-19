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

The argument is the board to list the images of, defaulting to the UNO Q.
`,
		Args: cobra.MaximumNArgs(1),
		Example: " " + os.Args[0] + " list\n" +
			" " + os.Args[0] + " list unoq\n",
		Run: func(cmd *cobra.Command, args []string) {
			boardID := string(registry.UnoQ.ID)
			if len(args) > 0 {
				boardID = args[0]
			}
			runListCommand(cmd.Context(), boardID, registry.Os(osStr))
		},
	}
	cmd.Flags().StringVar(&osStr, "os", "", "Distribution to list, for boards that publish more than one")
	return cmd
}

func runListCommand(ctx context.Context, boardID string, imageOs registry.Os) {
	board, ok := registry.BoardByID(registry.BoardID(boardID))
	if !ok {
		feedback.Fatal(i18n.Tr("%s is not a board, use one of: %s", boardID, strings.Join(registry.BoardIDs(), ", ")), feedback.ErrBadArgument)
	}

	client := registry.NewClient()

	// The whole index is listed: telling apart the boards inside it needs the
	// board each release carries, which is not decoded yet.
	manifest, err := client.GetInfoManifest(ctx, board.ResolveOs(imageOs))
	if err != nil {
		feedback.Fatal(i18n.Tr("error retrieving the manifest: %v", err), feedback.ErrBadArgument)
	}

	feedback.PrintResult(listResult{Latest: manifest.Latest, Releases: manifest.Releases})
}

type listResult struct {
	Latest   registry.Release   `json:"latest"`
	Releases []registry.Release `json:"releases"`
}

// Data implements Result interface
func (lr listResult) Data() interface{} {
	return lr
}

// String implements Result interface
func (lr listResult) String() string {
	t := table.NewWriter()
	t.SetStyle(tablestyle.CustomCleanStyle)
	t.AppendHeader(table.Row{"VERSION", "LATEST"})

	for i := len(lr.Releases) - 1; i >= 0; i-- {
		row := table.Row{lr.Releases[i].Version}
		if lr.Releases[i].Version == lr.Latest.Version {
			row = append(row, "✓")
		}
		t.AppendRow(row)
	}
	return t.Render()
}
