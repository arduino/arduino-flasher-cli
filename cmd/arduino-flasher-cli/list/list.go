// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package list

import (
	"context"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
	"github.com/arduino/arduino-flasher-cli/internal/registry"
	"github.com/arduino/arduino-flasher-cli/internal/tablestyle"
)

func NewListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the available Linux images",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			runListCommand(cmd.Context())
		},
	}
	return cmd
}

func runListCommand(ctx context.Context) {
	client := registry.NewClient()

	// TODO: add support for Ubuntu images
	manifest, err := client.GetInfoManifest(ctx, registry.Debian)
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
