// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package list

import (
	"context"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
	"go.bug.st/f"

	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
	"github.com/arduino/arduino-flasher-cli/internal/registry"
	"github.com/arduino/arduino-flasher-cli/internal/tablestyle"
)

func NewListCmd() *cobra.Command {
	var board string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the available Linux images",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			runListCommand(cmd.Context(), board)
		},
	}
	cmd.Flags().StringVar(&board, "board", "", "Filter by board type (unoq or ventunoq)")
	return cmd
}

func runListCommand(ctx context.Context, board string) {
	client := registry.NewClient()

	debianManifest, err := client.GetInfoManifest(ctx, registry.Debian)
	if err != nil {
		feedback.Fatal(i18n.Tr("error retrieving the manifest: %v", err), feedback.ErrBadArgument)
	}
	ubuntuManifest, err := client.GetInfoManifest(ctx, registry.Ubuntu)
	if err != nil {
		// TODO: temporary warning
		feedback.Warning(i18n.Tr("error retrieving the manifest: %v", err))
	}
	releases := debianManifest.Releases
	releases = append(releases, ubuntuManifest.Releases...)

	if board != "" {
		board = strings.ToLower(board)
		switch board {
		case "unoq":
			board = registry.UnoQ
		case "ventunoq":
			board = registry.VentunoQ
		default:
			feedback.Fatal(i18n.Tr("invalid board type: %s", board), feedback.ErrBadArgument)
		}
		releases = f.Filter(releases, func(r registry.Release) bool {
			return r.Board == board
		})
	}
	feedback.PrintResult(listResult{DebianLatest: debianManifest.Latest, UbuntuLatest: ubuntuManifest.Latest, Releases: releases})
}

type listResult struct {
	DebianLatest registry.Release   `json:"debian_latest"`
	UbuntuLatest registry.Release   `json:"ubuntu_latest"`
	Releases     []registry.Release `json:"releases"`
}

// Data implements Result interface
func (lr listResult) Data() interface{} {
	return lr
}

// String implements Result interface
func (lr listResult) String() string {
	t := table.NewWriter()
	t.SetStyle(tablestyle.CustomCleanStyle)
	t.AppendHeader(table.Row{"VERSION", "BOARD", "OS DISTRO", "LATEST"})

	for i := len(lr.Releases) - 1; i >= 0; i-- {
		row := table.Row{lr.Releases[i].Version, lr.Releases[i].Board, lr.Releases[i].OS}
		if (lr.Releases[i].Version == lr.DebianLatest.Version && strings.Contains(lr.Releases[i].OS, "Debian")) ||
			(lr.Releases[i].Version == lr.UbuntuLatest.Version && strings.Contains(lr.Releases[i].OS, "Ubuntu")) {
			row = append(row, "✓")
		}
		t.AppendRow(row)
	}
	return t.Render()
}
